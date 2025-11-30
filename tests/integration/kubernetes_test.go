package integration

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
	"github.com/tartarus-sandbox/tartarus/pkg/kubernetes/controllers"
)

func TestKubernetesIntegration(t *testing.T) {
	// Setup Scheme
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(tartarusv1alpha1.AddToScheme(scheme))

	// Mock Olympus Server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			// Respond with Accepted
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})

		case "/sandboxes/k8s-default-test-job":
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			// Respond with Running status
			status := map[string]interface{}{
				"id":      "k8s-default-test-job",
				"status":  domain.RunStatusRunning,
				"node_id": "test-node-1",
			}
			json.NewEncoder(w).Encode(status)

		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	// Setup Fake Client
	job := &tartarusv1alpha1.SandboxJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
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

	// Setup Reconciler
	reconciler := &controllers.SandboxJobReconciler{
		Client:      client,
		Scheme:      scheme,
		OlympusAddr: mockServer.URL,
		HTTPClient:  mockServer.Client(),
	}

	// 1. First Reconcile: Should submit to Olympus
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-job",
			Namespace: "default",
		},
	}

	res, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 2*time.Second, res.RequeueAfter)

	// Verify Status Update (ID should be set)
	updatedJob := &tartarusv1alpha1.SandboxJob{}
	err = client.Get(context.Background(), req.NamespacedName, updatedJob)
	require.NoError(t, err)
	assert.Equal(t, "k8s-default-test-job", updatedJob.Status.ID)
	assert.Equal(t, string(domain.RunStatusPending), updatedJob.Status.State)

	// 2. Second Reconcile: Should check status and update to Running
	res, err = reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, res.RequeueAfter)

	// Verify Status Update (State should be Running)
	err = client.Get(context.Background(), req.NamespacedName, updatedJob)
	require.NoError(t, err)
	assert.Equal(t, string(domain.RunStatusRunning), updatedJob.Status.State)
	assert.Equal(t, "test-node-1", updatedJob.Status.NodeID)
}
