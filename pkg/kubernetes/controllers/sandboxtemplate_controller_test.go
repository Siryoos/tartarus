/*
Copyright 2025 Tartarus Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tartarusv1alpha1 "github.com/tartarus-sandbox/tartarus/pkg/kubernetes/apis/tartarus/v1alpha1"
)

func TestSandboxTemplateReconciler_Reconcile(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(tartarusv1alpha1.AddToScheme(scheme))

	// Create initial object
	template := &tartarusv1alpha1.SandboxTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tartarus.io/v1alpha1",
			Kind:       "SandboxTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-template",
			Namespace: "default",
		},
		Spec: tartarusv1alpha1.SandboxTemplateSpec{
			Image: "alpine:latest",
		},
	}

	// Create fake client
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(template).
		WithStatusSubresource(&tartarusv1alpha1.SandboxTemplate{}).
		Build()

	// Create reconciler
	r := &SandboxTemplateReconciler{
		Client: k8sClient,
		Scheme: scheme,
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-template", Namespace: "default"}}

	// Reconcile
	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)

	// Verify status updated to Ready
	var updatedTemplate tartarusv1alpha1.SandboxTemplate
	err = k8sClient.Get(ctx, req.NamespacedName, &updatedTemplate)
	require.NoError(t, err)
	assert.True(t, updatedTemplate.Status.Ready)
	assert.Equal(t, "Template is valid", updatedTemplate.Status.Message)
}

func TestSandboxTemplateReconciler_Reconcile_Invalid(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(tartarusv1alpha1.AddToScheme(scheme))

	// Create initial object without image
	template := &tartarusv1alpha1.SandboxTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tartarus.io/v1alpha1",
			Kind:       "SandboxTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-template",
			Namespace: "default",
		},
		Spec: tartarusv1alpha1.SandboxTemplateSpec{
			Image: "", // Missing image
		},
	}

	// Create fake client
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(template).
		WithStatusSubresource(&tartarusv1alpha1.SandboxTemplate{}).
		Build()

	// Create reconciler
	r := &SandboxTemplateReconciler{
		Client: k8sClient,
		Scheme: scheme,
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "invalid-template", Namespace: "default"}}

	// Reconcile
	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)

	// Verify status updated to Not Ready
	var updatedTemplate tartarusv1alpha1.SandboxTemplate
	err = k8sClient.Get(ctx, req.NamespacedName, &updatedTemplate)
	require.NoError(t, err)
	assert.False(t, updatedTemplate.Status.Ready)
	assert.Equal(t, "Image is required", updatedTemplate.Status.Message)
}
