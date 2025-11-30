package domain

import (
	"time"
)

// IDs

type SandboxID string
type TemplateID string
type NodeID string
type SnapshotID string
type PolicyID string

// Statuses

type RunStatus string

const (
	RunStatusPending   RunStatus = "PENDING"
	RunStatusScheduled RunStatus = "SCHEDULED"
	RunStatusRunning   RunStatus = "RUNNING"
	RunStatusSucceeded RunStatus = "SUCCEEDED"
	RunStatusFailed    RunStatus = "FAILED"
	RunStatusCanceled  RunStatus = "CANCELED"
)

// Resources & instance profiles

type ResourceSpec struct {
	CPU     MilliCPU      `json:"cpu_milli"`
	Mem     Megabytes     `json:"mem_mb"`
	GPU     GPURequest    `json:"gpu,omitempty"`
	TTL     time.Duration `json:"ttl"`
	Profile string        `json:"profile"` // e.g. "phlegethon.large"
}

type MilliCPU int64
type Megabytes int64

type GPURequest struct {
	Count int    `json:"count"`
	Type  string `json:"type"` // vendor/model hint
}

// Network

type NetworkPolicyRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SandboxRequest is what Olympus enqueues into Acheron.

type SandboxRequest struct {
	ID         SandboxID         `json:"id"`
	Template   TemplateID        `json:"template"`
	NodeID     NodeID            `json:"node_id,omitempty"`    // Scheduled node
	HeatLevel  string            `json:"heat_level,omitempty"` // Phlegethon heat classification
	Command    []string          `json:"command"`
	Args       []string          `json:"args"`
	Env        map[string]string `json:"env"`
	Resources  ResourceSpec      `json:"resources"`
	NetworkRef NetworkPolicyRef  `json:"network"`
	Retention  RetentionPolicy   `json:"retention,omitempty"`
	Metadata   map[string]string `json:"metadata"` // tenant, user, origin, etc.
	CreatedAt  time.Time         `json:"created_at"`
}

// SandboxRun is the lifecycle instance of a request on a node.

type SandboxRun struct {
	ID          SandboxID  `json:"id"`
	RequestID   SandboxID  `json:"request_id"`
	NodeID      NodeID     `json:"node_id"`
	Template    TemplateID `json:"template"`
	Status      RunStatus  `json:"status"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  time.Time  `json:"finished_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	MemoryUsage Megabytes  `json:"memory_usage,omitempty"`
}

// Node & capacity

type ResourceCapacity struct {
	CPU MilliCPU  `json:"cpu_milli"`
	Mem Megabytes `json:"mem_mb"`
	GPU int       `json:"gpu"`
}

type NodeInfo struct {
	ID       NodeID            `json:"id"`
	Address  string            `json:"address"`
	Labels   map[string]string `json:"labels"`
	Capacity ResourceCapacity  `json:"capacity"`
}

type NodeStatus struct {
	NodeInfo
	Allocated       ResourceCapacity `json:"allocated"`
	Heartbeat       time.Time        `json:"heartbeat"`
	ActiveSandboxes []SandboxRun     `json:"active_sandboxes"`
}

// Template & snapshot references

type TemplateSpec struct {
	ID            TemplateID        `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	BaseImage     string            `json:"base_image"` // OCI ref or disk image ID
	KernelImage   string            `json:"kernel_image"`
	Resources     ResourceSpec      `json:"resources"`
	DefaultEnv    map[string]string `json:"default_env"`
	WarmupCommand []string          `json:"warmup_command,omitempty"`
}

type SnapshotRef struct {
	ID        SnapshotID `json:"id"`
	Template  TemplateID `json:"template"`
	CreatedAt time.Time  `json:"created_at"`
	Path      string     `json:"path"`
}

// Policies

type RetentionPolicy struct {
	MaxAge      time.Duration `json:"max_age"`
	KeepOutputs bool          `json:"keep_outputs"`
}

type SandboxPolicy struct {
	ID            PolicyID          `json:"id"`
	TemplateID    TemplateID        `json:"template_id"`
	Resources     ResourceSpec      `json:"resources"`
	NetworkPolicy NetworkPolicyRef  `json:"network"`
	Retention     RetentionPolicy   `json:"retention"`
	Tags          map[string]string `json:"tags"`
	Version       int64             `json:"version"`
}
