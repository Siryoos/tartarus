package thanatos

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
)

// ErrTerminationNotFound is returned when a termination ID doesn't exist.
var ErrTerminationNotFound = errors.New("termination not found")

// ErrTerminationAlreadyCancelled is returned when trying to cancel an already cancelled termination.
var ErrTerminationAlreadyCancelled = errors.New("termination already cancelled")

// ErrCheckpointNotFound is returned when a checkpoint doesn't exist.
var ErrCheckpointNotFound = errors.New("checkpoint not found")

// deferredTermination tracks a scheduled termination.
type deferredTermination struct {
	ID           string
	Request      *DeferredTerminationRequest
	ScheduledAt  time.Time
	Status       DeferredTerminationStatus
	CheckpointID string
	Result       *ControllerResult
	Error        error
	cancelFunc   context.CancelFunc
}

// DeferredScheduler manages scheduled terminations.
type DeferredScheduler struct {
	controller *ShutdownController
	hypnos     *hypnos.Manager

	mu           sync.RWMutex
	terminations map[string]*deferredTermination
	byBandbox    map[domain.SandboxID]string // sandboxID -> terminationID
}

// NewDeferredScheduler creates a new scheduler.
func NewDeferredScheduler(controller *ShutdownController, hypnos *hypnos.Manager) *DeferredScheduler {
	return &DeferredScheduler{
		controller:   controller,
		hypnos:       hypnos,
		terminations: make(map[string]*deferredTermination),
		byBandbox:    make(map[domain.SandboxID]string),
	}
}

// Schedule schedules a deferred termination.
func (s *DeferredScheduler) Schedule(ctx context.Context, req *DeferredTerminationRequest) (*DeferredTerminationResponse, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	terminationID := uuid.New().String()
	scheduledAt := time.Now().Add(req.Delay)

	dt := &deferredTermination{
		ID:          terminationID,
		Request:     req,
		ScheduledAt: scheduledAt,
		Status:      StatusPending,
	}

	s.mu.Lock()
	// Check if there's already a pending termination for this sandbox
	if existingID, exists := s.byBandbox[req.SandboxID]; exists {
		if existing, ok := s.terminations[existingID]; ok && existing.Status == StatusPending {
			s.mu.Unlock()
			return nil, errors.New("termination already scheduled for this sandbox")
		}
	}
	s.terminations[terminationID] = dt
	s.byBandbox[req.SandboxID] = terminationID
	s.mu.Unlock()

	// Schedule execution
	go s.executeDeferred(terminationID, req.Delay)

	return &DeferredTerminationResponse{
		TerminationID: terminationID,
		SandboxID:     req.SandboxID,
		ScheduledAt:   scheduledAt,
		Status:        StatusPending,
	}, nil
}

// executeDeferred waits for the delay and then executes termination.
func (s *DeferredScheduler) executeDeferred(terminationID string, delay time.Duration) {
	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	dt, ok := s.terminations[terminationID]
	if !ok {
		s.mu.Unlock()
		cancel()
		return
	}
	dt.cancelFunc = cancel
	s.mu.Unlock()

	// Wait for delay or cancellation
	select {
	case <-time.After(delay):
		// Proceed with termination
	case <-ctx.Done():
		// Cancelled
		return
	}

	// Update status
	s.mu.Lock()
	dt, ok = s.terminations[terminationID]
	if !ok || dt.Status != StatusPending {
		s.mu.Unlock()
		return
	}
	dt.Status = StatusInProgress
	s.mu.Unlock()

	// Build termination request
	termReq := &TerminationRequest{
		SandboxID:      dt.Request.SandboxID,
		TemplateID:     dt.Request.TemplateID,
		Reason:         mapStringToTerminationReason(dt.Request.Reason),
		ForceTimeout:   dt.Request.GracePeriod,
		SkipCheckpoint: !dt.Request.CreateCheckpoint,
	}

	// Execute termination
	result, err := s.controller.RequestTermination(ctx, termReq)

	// Update final status
	s.mu.Lock()
	dt = s.terminations[terminationID]
	if dt != nil {
		dt.Result = result
		dt.Error = err
		if err != nil {
			dt.Status = StatusFailed
		} else {
			dt.Status = StatusCompleted
			if result != nil && result.Checkpoint != "" {
				dt.CheckpointID = result.Checkpoint
			}
		}
	}
	s.mu.Unlock()
}

// mapStringToTerminationReason converts a string reason to TerminationReason.
func mapStringToTerminationReason(reason string) TerminationReason {
	switch reason {
	case "user_request", "user":
		return ReasonUserRequest
	case "policy_breach", "policy":
		return ReasonPolicyBreach
	case "resource_limit", "resources":
		return ReasonResourceLimit
	case "time_limit", "timeout":
		return ReasonTimeLimit
	case "system_shutdown", "shutdown":
		return ReasonSystemShutdown
	default:
		return ReasonUserRequest
	}
}

// Cancel cancels a pending termination.
func (s *DeferredScheduler) Cancel(terminationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dt, ok := s.terminations[terminationID]
	if !ok {
		return ErrTerminationNotFound
	}

	if dt.Status != StatusPending {
		if dt.Status == StatusCancelled {
			return ErrTerminationAlreadyCancelled
		}
		return errors.New("termination already in progress or completed")
	}

	dt.Status = StatusCancelled
	if dt.cancelFunc != nil {
		dt.cancelFunc()
	}

	return nil
}

// Get retrieves a termination by ID.
func (s *DeferredScheduler) Get(terminationID string) (*DeferredTerminationResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dt, ok := s.terminations[terminationID]
	if !ok {
		return nil, ErrTerminationNotFound
	}

	resp := &DeferredTerminationResponse{
		TerminationID: dt.ID,
		SandboxID:     dt.Request.SandboxID,
		ScheduledAt:   dt.ScheduledAt,
		Status:        dt.Status,
		CheckpointID:  dt.CheckpointID,
		Result:        dt.Result,
	}
	if dt.Error != nil {
		resp.ErrorMessage = dt.Error.Error()
	}

	return resp, nil
}

// GetBySandbox retrieves termination status by sandbox ID.
func (s *DeferredScheduler) GetBySandbox(sandboxID domain.SandboxID) (*DeferredTerminationResponse, error) {
	s.mu.RLock()
	terminationID, ok := s.byBandbox[sandboxID]
	s.mu.RUnlock()

	if !ok {
		return nil, ErrTerminationNotFound
	}

	return s.Get(terminationID)
}

// ListCheckpoints returns all available checkpoints.
func (s *DeferredScheduler) ListCheckpoints(_ context.Context, sandboxID domain.SandboxID) ([]CheckpointInfo, error) {
	if s.hypnos == nil {
		return nil, errors.New("hypnos manager not configured")
	}

	records := s.hypnos.List()
	var checkpoints []CheckpointInfo

	for _, rec := range records {
		// Filter by sandbox ID if provided
		if sandboxID != "" && rec.SandboxID != sandboxID {
			continue
		}

		checkpoints = append(checkpoints, CheckpointInfo{
			ID:         rec.SnapshotKey,
			SandboxID:  rec.SandboxID,
			TemplateID: domain.TemplateID(rec.Request.Template),
			CreatedAt:  rec.CreatedAt,
			Resumable:  true,
		})
	}

	return checkpoints, nil
}

// Resume resumes a sandbox from checkpoint.
func (s *DeferredScheduler) Resume(ctx context.Context, req *ResumeRequest) (*ResumeResponse, error) {
	if s.hypnos == nil {
		return nil, errors.New("hypnos manager not configured")
	}

	if req == nil || req.CheckpointID == "" {
		return nil, errors.New("checkpoint ID is required")
	}

	// Find the sandbox ID for this checkpoint
	records := s.hypnos.List()
	var sandboxID domain.SandboxID
	for _, rec := range records {
		if rec.SnapshotKey == req.CheckpointID {
			sandboxID = rec.SandboxID
			break
		}
	}

	if sandboxID == "" {
		return nil, ErrCheckpointNotFound
	}

	// Wake the sandbox
	run, err := s.hypnos.Wake(ctx, sandboxID)
	if err != nil {
		return nil, err
	}

	return &ResumeResponse{
		SandboxID:   run.ID,
		Status:      "resumed",
		ResumedFrom: req.CheckpointID,
		NodeID:      string(run.NodeID),
	}, nil
}

// Cleanup removes old completed/failed/cancelled terminations older than maxAge.
func (s *DeferredScheduler) Cleanup(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, dt := range s.terminations {
		if dt.Status != StatusPending && dt.Status != StatusInProgress {
			if dt.ScheduledAt.Before(cutoff) {
				delete(s.terminations, id)
				delete(s.byBandbox, dt.Request.SandboxID)
				removed++
			}
		}
	}

	return removed
}
