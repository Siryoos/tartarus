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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	tartarusv1alpha1 "github.com/tartarus-sandbox/tartarus/pkg/kubernetes/apis/tartarus/v1alpha1"
)

// SandboxJobReconciler reconciles a SandboxJob object
type SandboxJobReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	OlympusAddr string
	HTTPClient  *http.Client
}

//+kubebuilder:rbac:groups=tartarus.io,resources=sandboxjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=tartarus.io,resources=sandboxjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=tartarus.io,resources=sandboxjobs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SandboxJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var job tartarusv1alpha1.SandboxJob
	if err := r.Get(ctx, req.NamespacedName, &job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If the job is already completed or failed, we stop reconciling
	if job.Status.State == string(domain.RunStatusSucceeded) || job.Status.State == string(domain.RunStatusFailed) {
		return ctrl.Result{}, nil
	}

	// If ID is empty, we need to submit the job to Olympus
	if job.Status.ID == "" {
		logger.Info("Submitting new sandbox job to Olympus", "template", job.Spec.Template)

		// Parse retention duration
		var maxAge time.Duration
		if job.Spec.Retention.MaxAge != "" {
			var err error
			maxAge, err = time.ParseDuration(job.Spec.Retention.MaxAge)
			if err != nil {
				logger.Error(err, "Invalid retention duration", "duration", job.Spec.Retention.MaxAge)
				// Default to 0 or handle error? For now, log and proceed with 0
			}
		}

		// Create request
		sandboxReq := domain.SandboxRequest{
			ID:        domain.SandboxID(fmt.Sprintf("k8s-%s-%s", job.Namespace, job.Name)),
			Template:  domain.TemplateID(job.Spec.Template),
			Command:   job.Spec.Command,
			Args:      job.Spec.Args,
			Env:       job.Spec.Env,
			HeatLevel: job.Spec.HeatLevel,
			Resources: domain.ResourceSpec{
				CPU: domain.MilliCPU(job.Spec.Resources.CPU),
				Mem: domain.Megabytes(job.Spec.Resources.Memory),
			},
			NetworkRef: domain.NetworkPolicyRef{
				ID:   job.Spec.Network.ID,
				Name: job.Spec.Network.Name,
			},
			Retention: domain.RetentionPolicy{
				MaxAge:      maxAge,
				KeepOutputs: job.Spec.Retention.KeepOutputs,
			},
			Metadata:  job.Spec.Metadata,
			CreatedAt: time.Now(),
		}

		// Ensure metadata is initialized
		if sandboxReq.Metadata == nil {
			sandboxReq.Metadata = make(map[string]string)
		}
		sandboxReq.Metadata["k8s_namespace"] = job.Namespace
		sandboxReq.Metadata["k8s_name"] = job.Name

		// Submit to Olympus
		if err := r.submitToOlympus(ctx, &sandboxReq); err != nil {
			logger.Error(err, "Failed to submit job to Olympus")
			job.Status.State = string(domain.RunStatusFailed)
			job.Status.Message = fmt.Sprintf("Submission failed: %v", err)

			meta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
				Type:    string(tartarusv1alpha1.SandboxJobFailed),
				Status:  metav1.ConditionTrue,
				Reason:  "SubmissionFailed",
				Message: err.Error(),
			})

			if updateErr := r.Status().Update(ctx, &job); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}

		// Update status with ID
		job.Status.ID = string(sandboxReq.ID)
		job.Status.State = string(domain.RunStatusPending)
		job.Status.Message = "Submitted to Olympus"

		meta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
			Type:    string(tartarusv1alpha1.SandboxJobSubmitted),
			Status:  metav1.ConditionTrue,
			Reason:  "Submitted",
			Message: "Job submitted to Olympus",
		})

		if err := r.Status().Update(ctx, &job); err != nil {
			logger.Error(err, "Failed to update SandboxJob status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	// If ID is present, check status
	logger.Info("Checking status for sandbox", "id", job.Status.ID)
	status, err := r.checkStatus(ctx, job.Status.ID)
	if err != nil {
		logger.Error(err, "Failed to check status from Olympus")
		// Don't fail the job immediately, just retry
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Update CRD status
	job.Status.State = string(status.Status)
	job.Status.NodeID = string(status.NodeID)
	job.Status.Message = fmt.Sprintf("Updated at %s", time.Now().Format(time.RFC3339))

	switch status.Status {
	case domain.RunStatusPending:
		meta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
			Type:    string(tartarusv1alpha1.SandboxJobSubmitted),
			Status:  metav1.ConditionTrue,
			Reason:  "Pending",
			Message: "Waiting for scheduling",
		})
	case domain.RunStatusScheduled:
		meta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
			Type:    string(tartarusv1alpha1.SandboxJobScheduled),
			Status:  metav1.ConditionTrue,
			Reason:  "Scheduled",
			Message: fmt.Sprintf("Scheduled to node %s", status.NodeID),
		})
	case domain.RunStatusRunning:
		meta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
			Type:    string(tartarusv1alpha1.SandboxJobRunning),
			Status:  metav1.ConditionTrue,
			Reason:  "Running",
			Message: "Sandbox is running",
		})
	case domain.RunStatusSucceeded:
		meta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
			Type:    string(tartarusv1alpha1.SandboxJobCompleted),
			Status:  metav1.ConditionTrue,
			Reason:  "Succeeded",
			Message: "Sandbox completed successfully",
		})
	case domain.RunStatusFailed, domain.RunStatusCanceled:
		meta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
			Type:    string(tartarusv1alpha1.SandboxJobFailed),
			Status:  metav1.ConditionTrue,
			Reason:  "Failed",
			Message: fmt.Sprintf("Sandbox failed or canceled: %s", status.Status),
		})
	}

	if err := r.Status().Update(ctx, &job); err != nil {
		logger.Error(err, "Failed to update SandboxJob status")
		return ctrl.Result{}, err
	}

	// If terminal state, stop reconciling
	if status.Status == domain.RunStatusSucceeded || status.Status == domain.RunStatusFailed || status.Status == domain.RunStatusCanceled {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *SandboxJobReconciler) submitToOlympus(ctx context.Context, req *domain.SandboxRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/submit", r.OlympusAddr)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	r.addAuth(httpReq)

	resp, err := r.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("olympus returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Simplified status struct to match what we expect from Olympus
type olympusStatus struct {
	ID     string           `json:"id"`
	Status domain.RunStatus `json:"status"`
	NodeID string           `json:"node_id"`
}

func (r *SandboxJobReconciler) checkStatus(ctx context.Context, id string) (*olympusStatus, error) {
	url := fmt.Sprintf("%s/sandboxes/%s", r.OlympusAddr, id)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	r.addAuth(httpReq)

	resp, err := r.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("olympus returned %d", resp.StatusCode)
	}

	var st olympusStatus
	// Olympus returns SandboxRun, which has Status and NodeID
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, err
	}

	return &st, nil
}

func (r *SandboxJobReconciler) addAuth(req *http.Request) {
	if apiKey := os.Getenv("TARTARUS_API_KEY"); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *SandboxJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tartarusv1alpha1.SandboxJob{}).
		Complete(r)
}
