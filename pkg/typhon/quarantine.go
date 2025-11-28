package typhon

import (
	"context"
	"time"
)

// QuarantineManager handles high-risk workload isolation
type QuarantineManager interface {
	// Quarantine moves a sandbox to the quarantine pool
	Quarantine(ctx context.Context, req *QuarantineRequest) (*QuarantineRecord, error)

	// Release removes a sandbox from quarantine
	Release(ctx context.Context, sandboxID string, approval *ReleaseApproval) error

	// Examine analyzes a quarantined sandbox
	Examine(ctx context.Context, sandboxID string) (*ExaminationReport, error)

	// ListQuarantined returns all sandboxes in quarantine
	ListQuarantined(ctx context.Context, filter *QuarantineFilter) ([]*QuarantineRecord, error)

	// SetPolicy configures quarantine policies
	SetPolicy(ctx context.Context, policy *QuarantinePolicy) error
}

type QuarantineRequest struct {
	SandboxID      string
	Reason         QuarantineReason
	Evidence       []Evidence
	RequestedBy    string
	AutoQuarantine bool
}

type QuarantineReason string

const (
	ReasonSuspiciousBehavior Reason = "suspicious_behavior"
	ReasonPolicyViolation    Reason = "policy_violation"
	ReasonNetworkAnomaly     Reason = "network_anomaly"
	ReasonResourceAbuse      Reason = "resource_abuse"
	ReasonUntrustedSource    Reason = "untrusted_source"
	ReasonManualFlag         Reason = "manual_flag"
	ReasonSecurityScan       Reason = "security_scan"
)

// Alias for backward compatibility if needed, but using QuarantineReason is cleaner
type Reason = QuarantineReason

type Evidence struct {
	Type        EvidenceType
	Description string
	Data        []byte
	Timestamp   time.Time
}

type EvidenceType string

const (
	EvidenceTypeNetworkLog    EvidenceType = "network_log"
	EvidenceTypeSyscallTrace  EvidenceType = "syscall_trace"
	EvidenceTypeFileAccess    EvidenceType = "file_access"
	EvidenceTypeResourceSpike EvidenceType = "resource_spike"
	EvidenceTypeScreenshot    EvidenceType = "screenshot"
)

type QuarantineRecord struct {
	SandboxID        string
	Reason           QuarantineReason
	QuarantinedAt    time.Time
	QuarantinedBy    string
	Evidence         []Evidence
	Node             string // Dedicated quarantine node
	Status           QuarantineStatus
	ExaminationCount int
}

type QuarantineStatus string

const (
	StatusActive    QuarantineStatus = "active"
	StatusExamining QuarantineStatus = "examining"
	StatusReleased  QuarantineStatus = "released"
	StatusDestroyed QuarantineStatus = "destroyed"
)

type ExaminationReport struct {
	SandboxID      string
	ExaminedAt     time.Time
	Findings       []Finding
	RiskScore      float64 // 0.0 (safe) to 1.0 (dangerous)
	Recommendation RecommendedAction
}

type Finding struct {
	Severity    Severity
	Category    string
	Description string
	Evidence    *Evidence
}

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type RecommendedAction string

const (
	ActionRelease  RecommendedAction = "release"
	ActionMonitor  RecommendedAction = "monitor"
	ActionDestroy  RecommendedAction = "destroy"
	ActionEscalate RecommendedAction = "escalate"
)

type ReleaseApproval struct {
	ApprovedBy string
	Reason     string
	Conditions []string // Conditions for release
	ExpiresAt  *time.Time
}

type QuarantinePolicy struct {
	// Automatic quarantine triggers
	AutoTriggers []AutoQuarantineTrigger

	// Dedicated quarantine nodes
	QuarantineNodes []string

	// Enhanced isolation settings
	Isolation QuarantineIsolation

	// Retention period
	MaxQuarantineDuration time.Duration
}

type AutoQuarantineTrigger struct {
	Condition string // CEL expression
	Reason    QuarantineReason
	Enabled   bool
}

type QuarantineIsolation struct {
	// Network isolation
	NetworkMode   NetworkMode
	AllowedEgress []string // Only specific endpoints

	// Enhanced seccomp profile
	SeccompProfile string

	// Separate storage backend
	StorageBackend string

	// Additional monitoring
	EnableStrace  bool
	EnableAuditd  bool
	RecordNetwork bool
}

type NetworkMode string

const (
	NetworkModeNone       NetworkMode = "none"
	NetworkModeRestricted NetworkMode = "restricted"
	NetworkModeMonitored  NetworkMode = "monitored"
)

type QuarantineFilter struct {
	Status    QuarantineStatus
	Reason    QuarantineReason
	From      time.Time
	To        time.Time
	SandboxID string
}

// InMemoryQuarantineManager is a basic implementation for testing/dev
type InMemoryQuarantineManager struct {
	records map[string]*QuarantineRecord
	policy  *QuarantinePolicy
}

func NewInMemoryQuarantineManager() *InMemoryQuarantineManager {
	return &InMemoryQuarantineManager{
		records: make(map[string]*QuarantineRecord),
		policy:  &QuarantinePolicy{},
	}
}

func (m *InMemoryQuarantineManager) Quarantine(ctx context.Context, req *QuarantineRequest) (*QuarantineRecord, error) {
	record := &QuarantineRecord{
		SandboxID:     req.SandboxID,
		Reason:        req.Reason,
		QuarantinedAt: time.Now(),
		QuarantinedBy: req.RequestedBy,
		Evidence:      req.Evidence,
		Status:        StatusActive,
	}
	m.records[req.SandboxID] = record
	return record, nil
}

func (m *InMemoryQuarantineManager) Release(ctx context.Context, sandboxID string, approval *ReleaseApproval) error {
	if record, ok := m.records[sandboxID]; ok {
		record.Status = StatusReleased
		return nil
	}
	return nil
}

func (m *InMemoryQuarantineManager) Examine(ctx context.Context, sandboxID string) (*ExaminationReport, error) {
	return &ExaminationReport{
		SandboxID:      sandboxID,
		ExaminedAt:     time.Now(),
		Recommendation: ActionMonitor,
	}, nil
}

func (m *InMemoryQuarantineManager) ListQuarantined(ctx context.Context, filter *QuarantineFilter) ([]*QuarantineRecord, error) {
	var result []*QuarantineRecord
	for _, r := range m.records {
		if filter != nil {
			if filter.SandboxID != "" && r.SandboxID != filter.SandboxID {
				continue
			}
			if filter.Status != "" && r.Status != filter.Status {
				continue
			}
		}
		result = append(result, r)
	}
	return result, nil
}

func (m *InMemoryQuarantineManager) SetPolicy(ctx context.Context, policy *QuarantinePolicy) error {
	m.policy = policy
	return nil
}
