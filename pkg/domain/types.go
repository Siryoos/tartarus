package domain

import (
	"errors"
	"fmt"
	"strings"
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

// IsolationType defines the type of isolation/runtime to use for sandboxes.
type IsolationType string

const (
	IsolationMicroVM IsolationType = "microvm"
	IsolationWASM    IsolationType = "wasm"
	IsolationGVisor  IsolationType = "gvisor"
	IsolationAuto    IsolationType = "auto"
)

// Priority controls the scheduling order of sandbox requests in the job queue.
// Higher values are dequeued first. Producers should use the named constants
// rather than raw integers to remain forward-compatible.
type Priority int

const (
	PriorityLow    Priority = 0 // background / batch workloads
	PriorityNormal Priority = 1 // default for interactive requests
	PriorityHigh   Priority = 2 // latency-sensitive / SLA-bound requests
)

// Resources & instance profiles

type ResourceSpec struct {
	CPU     MilliCPU      `json:"cpu_milli"`
	Mem     Megabytes     `json:"mem_mb"`
	GPU     GPURequest    `json:"gpu,omitempty"`
	TTL     time.Duration `json:"ttl"`
	Profile Profile       `json:"profile"` // e.g. "phlegethon.large"
}

type MilliCPU int64
type Megabytes int64

type GPURequest struct {
	Count int    `json:"count"`
	Type  string `json:"type"` // vendor/model hint
}

// Profile represents a strongly-typed instance profile (e.g. "phlegethon.large").
type Profile string

// ParseProfile validates and parses a profile string.
func ParseProfile(s string) (Profile, error) {
	if s == "" {
		return "", errors.New("profile cannot be empty")
	}
	parts := strings.Split(s, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid profile format: %q (expected namespace.tier)", s)
	}
	if parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid profile format: %q", s)
	}
	return Profile(s), nil
}

// Namespace returns the namespace part of the profile (e.g., "phlegethon" from "phlegethon.large").
func (p Profile) Namespace() string {
	parts := strings.Split(string(p), ".")
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

// Tier returns the tier part of the profile (e.g., "large" from "phlegethon.large").
func (p Profile) Tier() string {
	parts := strings.Split(string(p), ".")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// Valid checks if the profile is correctly formatted.
func (p Profile) Valid() bool {
	_, err := ParseProfile(string(p))
	return err == nil
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
	Priority   Priority          `json:"priority,omitempty"`   // Scheduling priority; zero value is PriorityNormal
	Command    []string          `json:"command"`
	Args       []string          `json:"args"`
	Env        map[string]string `json:"env"`
	Resources  ResourceSpec      `json:"resources"`
	NetworkRef NetworkPolicyRef  `json:"network"`
	Retention  RetentionPolicy   `json:"retention,omitempty"`
	Secrets    map[string]string `json:"secrets,omitempty"`  // key -> secret ref
	Metadata   map[string]string `json:"metadata"`           // tenant, user, origin, etc.
	Hardened   bool              `json:"hardened,omitempty"` // Use hardened kernel/runtime
	CreatedAt  time.Time         `json:"created_at"`
}

// SandboxRun is the lifecycle instance of a request on a node.

type SandboxRun struct {
	ID          SandboxID         `json:"id"`
	RequestID   SandboxID         `json:"request_id"`
	NodeID      NodeID            `json:"node_id"`
	Template    TemplateID        `json:"template"`
	Status      RunStatus         `json:"status"`
	ExitCode    *int              `json:"exit_code,omitempty"`
	Error       string            `json:"error,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	MemoryUsage       Megabytes         `json:"memory_usage,omitempty"`
	MemoryUsageSource MemorySource      `json:"memory_source,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// MemorySource defines where the MemoryUsage metric came from.
type MemorySource string

const (
	MemorySourceUnknown      MemorySource = ""
	MemorySourceHostProc     MemorySource = "host_proc"    // RSS from /proc/<pid>/statm
	MemorySourceCgroupV2     MemorySource = "cgroup_v2"    // cgroup memory.current
	MemorySourceRunscStats   MemorySource = "runsc_stats"  // gVisor runsc events
	MemorySourceNotAvailable MemorySource = "not_available"// WASM or unsupported
)

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
