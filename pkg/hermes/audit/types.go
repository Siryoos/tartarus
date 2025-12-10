package audit

import (
	"time"
)

// Action represents the type of action being audited.
type Action string

const (
	ActionLogin      Action = "login"
	ActionLogout     Action = "logout"
	ActionCreate     Action = "create"
	ActionUpdate     Action = "update"
	ActionDelete     Action = "delete"
	ActionRead       Action = "read"
	ActionExecute    Action = "execute"
	ActionPermission Action = "permission"
	ActionTerminate  Action = "terminate"
)

// Result represents the outcome of the action.
type Result string

const (
	ResultSuccess Result = "success"
	ResultDenied  Result = "denied"
	ResultError   Result = "error"
)

// Resource represents the target of the action.
type Resource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Identity represents the actor performing the action.
type Identity struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // e.g., "user", "service_account", "api_key"
	TenantID string `json:"tenant_id,omitempty"`
}

// Event represents a single audit event.
type Event struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	Action       Action                 `json:"action"`
	Result       Result                 `json:"result"`
	Resource     Resource               `json:"resource"`
	Identity     *Identity              `json:"identity,omitempty"`
	SourceIP     string                 `json:"source_ip,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	RequestID    string                 `json:"request_id,omitempty"`
	Latency      time.Duration          `json:"latency,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`

	// PreviousHash is the hash of the previous event in the chain.
	// This is used for tamper-evidence.
	PreviousHash string `json:"previous_hash,omitempty"`
	// Hash is the hash of the current event (including PreviousHash).
	Hash string `json:"hash,omitempty"`
}
