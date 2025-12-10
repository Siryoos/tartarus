package thanatos

import (
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// DeferredTerminationRequest specifies parameters for scheduled termination.
type DeferredTerminationRequest struct {
	SandboxID        domain.SandboxID  `json:"sandbox_id"`
	TemplateID       domain.TemplateID `json:"template_id,omitempty"`
	Delay            time.Duration     `json:"delay,omitempty"`             // Time before termination starts (0 = immediate)
	GracePeriod      time.Duration     `json:"grace_period,omitempty"`      // Override policy grace (0 = use policy)
	CreateCheckpoint bool              `json:"create_checkpoint,omitempty"` // Create checkpoint before termination
	Reason           string            `json:"reason,omitempty"`            // User-provided reason
	RequestedBy      string            `json:"requested_by,omitempty"`      // Identity of requester
}

// DeferredTerminationStatus represents the state of a deferred termination.
type DeferredTerminationStatus string

const (
	// StatusPending indicates termination is scheduled but not yet started.
	StatusPending DeferredTerminationStatus = "pending"
	// StatusInProgress indicates termination is currently executing.
	StatusInProgress DeferredTerminationStatus = "in_progress"
	// StatusCompleted indicates termination finished successfully.
	StatusCompleted DeferredTerminationStatus = "completed"
	// StatusCancelled indicates termination was cancelled before execution.
	StatusCancelled DeferredTerminationStatus = "cancelled"
	// StatusFailed indicates termination encountered an error.
	StatusFailed DeferredTerminationStatus = "failed"
)

// DeferredTerminationResponse returned to API callers after scheduling termination.
type DeferredTerminationResponse struct {
	TerminationID string                    `json:"termination_id"`
	SandboxID     domain.SandboxID          `json:"sandbox_id"`
	ScheduledAt   time.Time                 `json:"scheduled_at"` // When termination will begin
	Status        DeferredTerminationStatus `json:"status"`
	CheckpointID  string                    `json:"checkpoint_id,omitempty"` // If checkpoint was created
	Result        *ControllerResult         `json:"result,omitempty"`        // Final result when completed
	ErrorMessage  string                    `json:"error_message,omitempty"`
}

// ResumeRequest specifies parameters for resuming a sandbox from checkpoint.
type ResumeRequest struct {
	CheckpointID string `json:"checkpoint_id"`           // Checkpoint to resume from
	OverrideNode string `json:"override_node,omitempty"` // Optional: specific node to resume on
}

// ResumeResponse returned after initiating resume from checkpoint.
type ResumeResponse struct {
	SandboxID   domain.SandboxID `json:"sandbox_id"`
	Status      string           `json:"status"`
	ResumedFrom string           `json:"resumed_from"` // Checkpoint ID
	NodeID      string           `json:"node_id,omitempty"`
}

// CheckpointInfo provides metadata about a resumable checkpoint.
type CheckpointInfo struct {
	ID         string            `json:"id"`
	SandboxID  domain.SandboxID  `json:"sandbox_id"`
	TemplateID domain.TemplateID `json:"template_id"`
	CreatedAt  time.Time         `json:"created_at"`
	Size       int64             `json:"size,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	Resumable  bool              `json:"resumable"`
}

// TerminationAPIRequest is the HTTP request body for termination endpoints.
type TerminationAPIRequest struct {
	Delay            string `json:"delay,omitempty"` // Duration string (e.g., "5m", "30s")
	GracePeriod      string `json:"grace_period,omitempty"`
	CreateCheckpoint bool   `json:"create_checkpoint,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

// TerminationAPIResponse is the HTTP response body for termination endpoints.
type TerminationAPIResponse struct {
	TerminationID string `json:"termination_id,omitempty"`
	SandboxID     string `json:"sandbox_id"`
	Status        string `json:"status"`
	ScheduledAt   string `json:"scheduled_at,omitempty"`
	CheckpointID  string `json:"checkpoint_id,omitempty"`
	Message       string `json:"message,omitempty"`
}

// ResumeAPIRequest is the HTTP request body for resume endpoints.
type ResumeAPIRequest struct {
	CheckpointID string `json:"checkpoint_id"`
	OverrideNode string `json:"override_node,omitempty"`
}

// ResumeAPIResponse is the HTTP response body for resume endpoints.
type ResumeAPIResponse struct {
	SandboxID   string `json:"sandbox_id"`
	Status      string `json:"status"`
	ResumedFrom string `json:"resumed_from"`
	Message     string `json:"message,omitempty"`
}
