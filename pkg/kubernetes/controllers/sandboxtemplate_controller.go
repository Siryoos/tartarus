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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tartarusv1alpha1 "github.com/tartarus-sandbox/tartarus/pkg/kubernetes/apis/tartarus/v1alpha1"
)

// SandboxTemplateReconciler reconciles a SandboxTemplate object
type SandboxTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=tartarus.io,resources=sandboxtemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=tartarus.io,resources=sandboxtemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=tartarus.io,resources=sandboxtemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SandboxTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var template tartarusv1alpha1.SandboxTemplate
	if err := r.Get(ctx, req.NamespacedName, &template); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// For now, we just mark the template as ready if the Image is specified
	// In a real implementation, we might want to validate the image or pre-pull it
	if template.Spec.Image == "" {
		logger.Info("SandboxTemplate has no image specified", "name", template.Name)
		template.Status.Ready = false
		template.Status.Message = "Image is required"
	} else {
		template.Status.Ready = true
		template.Status.Message = "Template is valid"
	}

	if err := r.Status().Update(ctx, &template); err != nil {
		logger.Error(err, "Failed to update SandboxTemplate status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SandboxTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tartarusv1alpha1.SandboxTemplate{}).
		Complete(r)
}
