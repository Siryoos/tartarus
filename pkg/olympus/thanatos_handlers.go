package olympus

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/thanatos"
)

// ThanatosHandlers provides HTTP handlers for graceful termination API.
type ThanatosHandlers struct {
	scheduler *thanatos.DeferredScheduler
	logger    hermes.Logger
}

// NewThanatosHandlers creates new Thanatos HTTP handlers.
func NewThanatosHandlers(scheduler *thanatos.DeferredScheduler, logger hermes.Logger) *ThanatosHandlers {
	return &ThanatosHandlers{
		scheduler: scheduler,
		logger:    logger,
	}
}

// HandleTerminate handles POST/GET/DELETE /sandboxes/{id}/terminate
func (h *ThanatosHandlers) HandleTerminate(w http.ResponseWriter, r *http.Request) {
	// Extract sandbox ID from path: /sandboxes/terminate/{id}
	path := r.URL.Path
	sandboxID := extractSandboxID(path, "/sandboxes/terminate/")
	if sandboxID == "" {
		http.Error(w, "Missing sandbox ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.handleScheduleTermination(w, r, domain.SandboxID(sandboxID))
	case http.MethodGet:
		h.handleGetTerminationStatus(w, r, domain.SandboxID(sandboxID))
	case http.MethodDelete:
		h.handleCancelTermination(w, r, domain.SandboxID(sandboxID))
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ThanatosHandlers) handleScheduleTermination(w http.ResponseWriter, r *http.Request, sandboxID domain.SandboxID) {
	var apiReq thanatos.TerminationAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil && err.Error() != "EOF" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Parse durations
	var delay, grace time.Duration
	if apiReq.Delay != "" {
		d, err := time.ParseDuration(apiReq.Delay)
		if err != nil {
			http.Error(w, "Invalid delay format", http.StatusBadRequest)
			return
		}
		delay = d
	}
	if apiReq.GracePeriod != "" {
		g, err := time.ParseDuration(apiReq.GracePeriod)
		if err != nil {
			http.Error(w, "Invalid grace_period format", http.StatusBadRequest)
			return
		}
		grace = g
	}

	req := &thanatos.DeferredTerminationRequest{
		SandboxID:        sandboxID,
		Delay:            delay,
		GracePeriod:      grace,
		CreateCheckpoint: apiReq.CreateCheckpoint,
		Reason:           apiReq.Reason,
	}

	resp, err := h.scheduler.Schedule(r.Context(), req)
	if err != nil {
		h.logger.Error(r.Context(), "Failed to schedule termination", map[string]any{
			"sandbox_id": sandboxID,
			"error":      err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apiResp := thanatos.TerminationAPIResponse{
		TerminationID: resp.TerminationID,
		SandboxID:     string(resp.SandboxID),
		Status:        string(resp.Status),
		ScheduledAt:   resp.ScheduledAt.Format(time.RFC3339),
		Message:       "Termination scheduled",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(apiResp)
}

func (h *ThanatosHandlers) handleGetTerminationStatus(w http.ResponseWriter, r *http.Request, sandboxID domain.SandboxID) {
	resp, err := h.scheduler.GetBySandbox(sandboxID)
	if err != nil {
		if err == thanatos.ErrTerminationNotFound {
			http.Error(w, "No termination found for sandbox", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apiResp := thanatos.TerminationAPIResponse{
		TerminationID: resp.TerminationID,
		SandboxID:     string(resp.SandboxID),
		Status:        string(resp.Status),
		ScheduledAt:   resp.ScheduledAt.Format(time.RFC3339),
		CheckpointID:  resp.CheckpointID,
	}
	if resp.ErrorMessage != "" {
		apiResp.Message = resp.ErrorMessage
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiResp)
}

func (h *ThanatosHandlers) handleCancelTermination(w http.ResponseWriter, r *http.Request, sandboxID domain.SandboxID) {
	// First get the termination ID for this sandbox
	resp, err := h.scheduler.GetBySandbox(sandboxID)
	if err != nil {
		if err == thanatos.ErrTerminationNotFound {
			http.Error(w, "No termination found for sandbox", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.scheduler.Cancel(resp.TerminationID); err != nil {
		if err == thanatos.ErrTerminationAlreadyCancelled {
			http.Error(w, "Termination already cancelled", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "cancelled",
		"message": "Termination cancelled successfully",
	})
}

// HandleCheckpoints handles GET /sandboxes/{id}/checkpoints
func (h *ThanatosHandlers) HandleCheckpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract sandbox ID from path: /sandboxes/checkpoints/{id}
	path := r.URL.Path
	sandboxID := extractSandboxID(path, "/sandboxes/checkpoints/")

	checkpoints, err := h.scheduler.ListCheckpoints(r.Context(), domain.SandboxID(sandboxID))
	if err != nil {
		h.logger.Error(r.Context(), "Failed to list checkpoints", map[string]any{
			"sandbox_id": sandboxID,
			"error":      err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checkpoints)
}

// HandleResume handles POST /sandboxes/resume
func (h *ThanatosHandlers) HandleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var apiReq thanatos.ResumeAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if apiReq.CheckpointID == "" {
		http.Error(w, "checkpoint_id is required", http.StatusBadRequest)
		return
	}

	req := &thanatos.ResumeRequest{
		CheckpointID: apiReq.CheckpointID,
		OverrideNode: apiReq.OverrideNode,
	}

	resp, err := h.scheduler.Resume(r.Context(), req)
	if err != nil {
		if err == thanatos.ErrCheckpointNotFound {
			http.Error(w, "Checkpoint not found", http.StatusNotFound)
			return
		}
		h.logger.Error(r.Context(), "Failed to resume from checkpoint", map[string]any{
			"checkpoint_id": apiReq.CheckpointID,
			"error":         err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apiResp := thanatos.ResumeAPIResponse{
		SandboxID:   string(resp.SandboxID),
		Status:      resp.Status,
		ResumedFrom: resp.ResumedFrom,
		Message:     "Sandbox resumed from checkpoint",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(apiResp)
}

// extractSandboxID extracts the sandbox ID from a URL path with a given prefix.
func extractSandboxID(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	id := path[len(prefix):]
	// Remove trailing slash if present
	id = strings.TrimSuffix(id, "/")
	return id
}

// RegisterRoutes registers all Thanatos routes on the given mux.
func (h *ThanatosHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/sandboxes/terminate/", h.HandleTerminate)
	mux.HandleFunc("/sandboxes/checkpoints/", h.HandleCheckpoints)
	mux.HandleFunc("/sandboxes/resume", h.HandleResume)
}

// For convenience, add methods to Manager that wrap the scheduler

// TerminateSandboxGracefully schedules graceful termination via Thanatos.
func (m *Manager) TerminateSandboxGracefully(ctx context.Context, scheduler *thanatos.DeferredScheduler, id domain.SandboxID, opts thanatos.DeferredTerminationRequest) (*thanatos.DeferredTerminationResponse, error) {
	// Get sandbox info for template
	run, err := m.Hades.GetRun(ctx, id)
	if err != nil {
		return nil, ErrSandboxNotFound
	}

	opts.SandboxID = id
	opts.TemplateID = run.Template

	resp, err := scheduler.Schedule(ctx, &opts)
	if err != nil {
		m.Logger.Error(ctx, "Failed to schedule graceful termination", map[string]any{
			"sandbox_id": id,
			"error":      err.Error(),
		})
		return nil, err
	}

	m.Logger.Info(ctx, "Graceful termination scheduled", map[string]any{
		"sandbox_id":     id,
		"termination_id": resp.TerminationID,
		"scheduled_at":   resp.ScheduledAt,
	})

	return resp, nil
}

// ResumeSandboxFromCheckpoint resumes a sandbox from a checkpoint.
func (m *Manager) ResumeSandboxFromCheckpoint(ctx context.Context, scheduler *thanatos.DeferredScheduler, checkpointID string) (*thanatos.ResumeResponse, error) {
	resp, err := scheduler.Resume(ctx, &thanatos.ResumeRequest{
		CheckpointID: checkpointID,
	})
	if err != nil {
		m.Logger.Error(ctx, "Failed to resume from checkpoint", map[string]any{
			"checkpoint_id": checkpointID,
			"error":         err.Error(),
		})
		return nil, err
	}

	m.Logger.Info(ctx, "Sandbox resumed from checkpoint", map[string]any{
		"sandbox_id":    resp.SandboxID,
		"checkpoint_id": checkpointID,
	})

	return resp, nil
}
