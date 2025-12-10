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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	tartarusv1alpha1 "github.com/tartarus-sandbox/tartarus/pkg/kubernetes/apis/tartarus/v1alpha1"
)

func TestSandboxJobReconciler_Reconcile(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(tartarusv1alpha1.AddToScheme(scheme))

	// Setup mock Olympus server
	olympusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submit":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var req domain.SandboxRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Verify fields
			if len(req.Args) != 1 || req.Args[0] != "world" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.Env["FOO"] != "bar" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.NetworkRef.ID != "default-policy" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.HeatLevel != "ember" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.Retention.MaxAge != time.Hour {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.Metadata["custom"] != "value" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.Metadata["k8s_namespace"] != "default" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]string{"id": string(req.ID)})

		case "/sandboxes/k8s-default-test-job":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			// Simulate running status
			status := domain.SandboxRun{
				ID:       "k8s-default-test-job",
				Status:   domain.RunStatusRunning,
				NodeID:   "node-1",
				Template: "alpine",
			}
			json.NewEncoder(w).Encode(status)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer olympusServer.Close()

	// Create initial object
	job := &tartarusv1alpha1.SandboxJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tartarus.io/v1alpha1",
			Kind:       "SandboxJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: tartarusv1alpha1.SandboxJobSpec{
			Template: "alpine",
			Command:  []string{"echo", "hello"},
			Args:     []string{"world"},
			Env:      map[string]string{"FOO": "bar"},
			Resources: tartarusv1alpha1.ResourceSpec{
				CPU:    1000,
				Memory: 128,
			},
			Network: tartarusv1alpha1.NetworkPolicyRef{
				ID: "default-policy",
			},
			HeatLevel: "ember",
			Retention: tartarusv1alpha1.RetentionPolicy{
				MaxAge:      "1h",
				KeepOutputs: true,
			},
			Metadata: map[string]string{
				"custom": "value",
			},
		},
	}

	// Create fake client
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(job).
		WithStatusSubresource(&tartarusv1alpha1.SandboxJob{}).
		Build()

	// Create reconciler
	r := &SandboxJobReconciler{
		Client:      k8sClient,
		Scheme:      scheme,
		OlympusAddr: olympusServer.URL,
		HTTPClient:  olympusServer.Client(),
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-job", Namespace: "default"}}

	// 1. First reconciliation: Should submit to Olympus
	res, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 2*time.Second, res.RequeueAfter)

	// Verify status updated to Pending with ID
	var updatedJob tartarusv1alpha1.SandboxJob
	err = k8sClient.Get(ctx, req.NamespacedName, &updatedJob)
	require.NoError(t, err)
	assert.NotEmpty(t, updatedJob.Status.ID)
	assert.Equal(t, "k8s-default-test-job", updatedJob.Status.ID)
	assert.Equal(t, string(domain.RunStatusPending), updatedJob.Status.State)

	// Check Condition
	require.Len(t, updatedJob.Status.Conditions, 1)
	assert.Equal(t, string(tartarusv1alpha1.SandboxJobSubmitted), updatedJob.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, updatedJob.Status.Conditions[0].Status)

	// 2. Second reconciliation: Should check status and update to Running
	res, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, res.RequeueAfter)

	// Verify status updated to Running
	err = k8sClient.Get(ctx, req.NamespacedName, &updatedJob)
	require.NoError(t, err)
	assert.Equal(t, string(domain.RunStatusRunning), updatedJob.Status.State)
	assert.Equal(t, "node-1", updatedJob.Status.NodeID)

	// Check Condition
	// Depending on implementation, we might have multiple conditions or just the latest one if types differ
	// Since we use SetStatusCondition with different types, they should accumulate or update existing
	// Here we expect Running condition to be added/updated.
	// We might have Submitted still there.
	found := false
	for _, cond := range updatedJob.Status.Conditions {
		if cond.Type == string(tartarusv1alpha1.SandboxJobRunning) {
			assert.Equal(t, metav1.ConditionTrue, cond.Status)
			found = true
			break
		}
	}
	assert.True(t, found, "Expected SandboxJobRunning condition")
}
