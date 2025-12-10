package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
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
	"github.com/tartarus-sandbox/tartarus/pkg/kubernetes/controllers"
)

// TestSandboxJobE2E_FullLifecycle tests the complete SandboxJob lifecycle
// from creation through scheduled, running, to succeeded state.
func TestSandboxJobE2E_FullLifecycle(t *testing.T) {
	scheme := setupScheme()

	// Track lifecycle states - starting at SCHEDULED since PENDING is set locally on submit
	var mu sync.Mutex
	lifecycleStates := []domain.RunStatus{
		domain.RunStatusScheduled,
		domain.RunStatusRunning,
		domain.RunStatusSucceeded,
	}
	currentStateIdx := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/submit":
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req domain.SandboxRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})

		case "/sandboxes/k8s-default-lifecycle-test":
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			// Return the current state, then advance for next call
			status := map[string]interface{}{
				"id":      "k8s-default-lifecycle-test",
				"status":  lifecycleStates[currentStateIdx],
				"node_id": "test-node-1",
			}
			json.NewEncoder(w).Encode(status)
			// Advance state for next call
			if currentStateIdx < len(lifecycleStates)-1 {
				currentStateIdx++
			}

		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	job := &tartarusv1alpha1.SandboxJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lifecycle-test",
			Namespace: "default",
		},
		Spec: tartarusv1alpha1.SandboxJobSpec{
			Template: "alpine-base",
			Command:  []string{"echo", "hello"},
			Resources: tartarusv1alpha1.ResourceSpec{
				CPU:    100,
				Memory: 128,
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&tartarusv1alpha1.SandboxJob{}).
		WithObjects(job).
		Build()

	reconciler := &controllers.SandboxJobReconciler{
		Client:      client,
		Scheme:      scheme,
		OlympusAddr: mockServer.URL,
		HTTPClient:  mockServer.Client(),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "lifecycle-test",
			Namespace: "default",
		},
	}
	ctx := context.Background()

	// Phase 1: Submit
	res, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.NotZero(t, res.RequeueAfter, "Should requeue after submission")

	updatedJob := &tartarusv1alpha1.SandboxJob{}
	err = client.Get(ctx, req.NamespacedName, updatedJob)
	require.NoError(t, err)
	assert.Equal(t, "k8s-default-lifecycle-test", updatedJob.Status.ID)
	assert.Equal(t, string(domain.RunStatusPending), updatedJob.Status.State)

	// Phase 2: Scheduled
	res, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	err = client.Get(ctx, req.NamespacedName, updatedJob)
	require.NoError(t, err)
	assert.Equal(t, string(domain.RunStatusScheduled), updatedJob.Status.State)

	// Phase 3: Running
	res, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	err = client.Get(ctx, req.NamespacedName, updatedJob)
	require.NoError(t, err)
	assert.Equal(t, string(domain.RunStatusRunning), updatedJob.Status.State)

	// Phase 4: Succeeded (terminal)
	res, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter, "Should not requeue after terminal state")
	err = client.Get(ctx, req.NamespacedName, updatedJob)
	require.NoError(t, err)
	assert.Equal(t, string(domain.RunStatusSucceeded), updatedJob.Status.State)

	// Verify condition is set
	var foundCompleted bool
	for _, cond := range updatedJob.Status.Conditions {
		if cond.Type == string(tartarusv1alpha1.SandboxJobCompleted) {
			assert.Equal(t, metav1.ConditionTrue, cond.Status)
			foundCompleted = true
		}
	}
	assert.True(t, foundCompleted, "Expected Completed condition")
}

// TestSandboxJobE2E_Overhead measures reconciliation overhead and asserts <1s SLO.
func TestSandboxJobE2E_Overhead(t *testing.T) {
	scheme := setupScheme()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submit":
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
		case "/sandboxes/k8s-default-overhead-test":
			status := map[string]interface{}{
				"id":      "k8s-default-overhead-test",
				"status":  domain.RunStatusRunning,
				"node_id": "node-1",
			}
			json.NewEncoder(w).Encode(status)
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	job := &tartarusv1alpha1.SandboxJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "overhead-test",
			Namespace: "default",
		},
		Spec: tartarusv1alpha1.SandboxJobSpec{
			Template: "alpine-base",
			Command:  []string{"sleep", "1"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&tartarusv1alpha1.SandboxJob{}).
		WithObjects(job).
		Build()

	reconciler := &controllers.SandboxJobReconciler{
		Client:      client,
		Scheme:      scheme,
		OlympusAddr: mockServer.URL,
		HTTPClient:  mockServer.Client(),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "overhead-test",
			Namespace: "default",
		},
	}
	ctx := context.Background()

	// Measure submission overhead
	start := time.Now()
	_, err := reconciler.Reconcile(ctx, req)
	submitOverhead := time.Since(start)
	require.NoError(t, err)
	assert.Less(t, submitOverhead, 1*time.Second, "Submission reconciliation overhead must be <1s")
	t.Logf("Submit reconcile overhead: %v", submitOverhead)

	// Measure status check overhead
	start = time.Now()
	_, err = reconciler.Reconcile(ctx, req)
	statusOverhead := time.Since(start)
	require.NoError(t, err)
	assert.Less(t, statusOverhead, 1*time.Second, "Status check reconciliation overhead must be <1s")
	t.Logf("Status check reconcile overhead: %v", statusOverhead)
}

// TestSandboxJobE2E_FailedJob tests failure handling and condition updates.
func TestSandboxJobE2E_FailedJob(t *testing.T) {
	scheme := setupScheme()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submit":
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
		case "/sandboxes/k8s-default-failed-test":
			status := map[string]interface{}{
				"id":      "k8s-default-failed-test",
				"status":  domain.RunStatusFailed,
				"node_id": "node-1",
			}
			json.NewEncoder(w).Encode(status)
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	job := &tartarusv1alpha1.SandboxJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failed-test",
			Namespace: "default",
		},
		Spec: tartarusv1alpha1.SandboxJobSpec{
			Template: "alpine-base",
			Command:  []string{"exit", "1"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&tartarusv1alpha1.SandboxJob{}).
		WithObjects(job).
		Build()

	reconciler := &controllers.SandboxJobReconciler{
		Client:      client,
		Scheme:      scheme,
		OlympusAddr: mockServer.URL,
		HTTPClient:  mockServer.Client(),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "failed-test",
			Namespace: "default",
		},
	}
	ctx := context.Background()

	// Submit
	_, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	// Get failed status
	res, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter, "Should not requeue after failure")

	updatedJob := &tartarusv1alpha1.SandboxJob{}
	err = client.Get(ctx, req.NamespacedName, updatedJob)
	require.NoError(t, err)
	assert.Equal(t, string(domain.RunStatusFailed), updatedJob.Status.State)

	// Verify failed condition
	var foundFailed bool
	for _, cond := range updatedJob.Status.Conditions {
		if cond.Type == string(tartarusv1alpha1.SandboxJobFailed) {
			assert.Equal(t, metav1.ConditionTrue, cond.Status)
			foundFailed = true
		}
	}
	assert.True(t, foundFailed, "Expected Failed condition")
}

// TestSandboxTemplateE2E_Validation tests template validation behavior.
func TestSandboxTemplateE2E_Validation(t *testing.T) {
	scheme := setupScheme()

	tests := []struct {
		name          string
		template      *tartarusv1alpha1.SandboxTemplate
		expectedReady bool
		expectedMsg   string
	}{
		{
			name: "valid template with image",
			template: &tartarusv1alpha1.SandboxTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-template",
					Namespace: "default",
				},
				Spec: tartarusv1alpha1.SandboxTemplateSpec{
					Image: "python:3.11-slim",
				},
			},
			expectedReady: true,
			expectedMsg:   "Template is valid",
		},
		{
			name: "invalid template without image",
			template: &tartarusv1alpha1.SandboxTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-template",
					Namespace: "default",
				},
				Spec: tartarusv1alpha1.SandboxTemplateSpec{
					// No image specified
				},
			},
			expectedReady: false,
			expectedMsg:   "Image is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&tartarusv1alpha1.SandboxTemplate{}).
				WithObjects(tt.template).
				Build()

			reconciler := &controllers.SandboxTemplateReconciler{
				Client: client,
				Scheme: scheme,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.template.Name,
					Namespace: tt.template.Namespace,
				},
			}

			_, err := reconciler.Reconcile(context.Background(), req)
			require.NoError(t, err)

			updatedTemplate := &tartarusv1alpha1.SandboxTemplate{}
			err = client.Get(context.Background(), req.NamespacedName, updatedTemplate)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedReady, updatedTemplate.Status.Ready)
			assert.Equal(t, tt.expectedMsg, updatedTemplate.Status.Message)
		})
	}
}

// TestMultiTenantIsolation verifies that jobs in different namespaces are isolated.
func TestMultiTenantIsolation(t *testing.T) {
	scheme := setupScheme()

	// Track which jobs were submitted with which tenant metadata
	var mu sync.Mutex
	submittedJobs := make(map[string]string) // sandbox ID -> tenant/namespace

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.URL.Path == "/submit" {
			var req domain.SandboxRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			submittedJobs[string(req.ID)] = req.Metadata["k8s_namespace"]
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
			return
		}

		// Handle status checks
		if len(r.URL.Path) > len("/sandboxes/") {
			sandboxID := r.URL.Path[len("/sandboxes/"):]
			status := map[string]interface{}{
				"id":      sandboxID,
				"status":  domain.RunStatusRunning,
				"node_id": "node-1",
			}
			json.NewEncoder(w).Encode(status)
			return
		}

		http.Error(w, "Not found", http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Create jobs in two different namespaces (tenants)
	jobTenantA := &tartarusv1alpha1.SandboxJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-a",
			Namespace: "tenant-a",
		},
		Spec: tartarusv1alpha1.SandboxJobSpec{
			Template: "alpine-base",
			Command:  []string{"echo", "tenant-a"},
		},
	}

	jobTenantB := &tartarusv1alpha1.SandboxJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-b",
			Namespace: "tenant-b",
		},
		Spec: tartarusv1alpha1.SandboxJobSpec{
			Template: "alpine-base",
			Command:  []string{"echo", "tenant-b"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&tartarusv1alpha1.SandboxJob{}).
		WithObjects(jobTenantA, jobTenantB).
		Build()

	reconciler := &controllers.SandboxJobReconciler{
		Client:      client,
		Scheme:      scheme,
		OlympusAddr: mockServer.URL,
		HTTPClient:  mockServer.Client(),
	}

	ctx := context.Background()

	// Reconcile tenant-a job
	reqA := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job-a",
			Namespace: "tenant-a",
		},
	}
	_, err := reconciler.Reconcile(ctx, reqA)
	require.NoError(t, err)

	// Reconcile tenant-b job
	reqB := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job-b",
			Namespace: "tenant-b",
		},
	}
	_, err = reconciler.Reconcile(ctx, reqB)
	require.NoError(t, err)

	// Verify tenant metadata was correctly set
	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, "tenant-a", submittedJobs["k8s-tenant-a-job-a"],
		"Job A should have tenant-a namespace in metadata")
	assert.Equal(t, "tenant-b", submittedJobs["k8s-tenant-b-job-b"],
		"Job B should have tenant-b namespace in metadata")

	// Verify jobs have different IDs (namespace isolation)
	assert.NotEqual(t, submittedJobs["k8s-tenant-a-job-a"], submittedJobs["k8s-tenant-b-job-b"],
		"Jobs in different tenants should have different identities")
}

// TestSandboxJobE2E_SubmissionError tests handling of Olympus submission errors.
func TestSandboxJobE2E_SubmissionError(t *testing.T) {
	scheme := setupScheme()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/submit" {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Not found", http.StatusNotFound)
	}))
	defer mockServer.Close()

	job := &tartarusv1alpha1.SandboxJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "error-test",
			Namespace: "default",
		},
		Spec: tartarusv1alpha1.SandboxJobSpec{
			Template: "alpine-base",
			Command:  []string{"echo", "hello"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&tartarusv1alpha1.SandboxJob{}).
		WithObjects(job).
		Build()

	reconciler := &controllers.SandboxJobReconciler{
		Client:      client,
		Scheme:      scheme,
		OlympusAddr: mockServer.URL,
		HTTPClient:  mockServer.Client(),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "error-test",
			Namespace: "default",
		},
	}

	// Should handle error gracefully
	res, err := reconciler.Reconcile(context.Background(), req)
	require.Error(t, err)
	assert.NotZero(t, res.RequeueAfter, "Should requeue after error")

	// Verify status shows failure
	updatedJob := &tartarusv1alpha1.SandboxJob{}
	err = client.Get(context.Background(), req.NamespacedName, updatedJob)
	require.NoError(t, err)
	assert.Equal(t, string(domain.RunStatusFailed), updatedJob.Status.State)
	assert.Contains(t, updatedJob.Status.Message, "Submission failed")
}

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(tartarusv1alpha1.AddToScheme(scheme))
	return scheme
}
