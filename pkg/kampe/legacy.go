package kampe

import (
	"context"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// LegacyRuntime wraps legacy container runtimes with the Tartarus interface
type LegacyRuntime interface {
	tartarus.SandboxRuntime

	// Migration helpers
	CanMigrate(ctx context.Context, containerID string) (bool, error)
	MigrateToMicroVM(ctx context.Context, containerID string) (*MigrationPlan, error)
	ExportState(ctx context.Context, containerID string) (*ContainerState, error)
}

// MigrationPlan describes steps to move from container to microVM
type MigrationPlan struct {
	ContainerID       string
	TargetTemplate    string
	RequiredChanges   []MigrationChange
	EstimatedDowntime time.Duration
	RiskLevel         RiskLevel
	Recommendations   []string
}

type MigrationChange struct {
	Type        ChangeType
	Description string
	Required    bool
	AutoFix     bool
}

type ChangeType string

const (
	ChangeTypeFilesystem  ChangeType = "filesystem"
	ChangeTypeNetwork     ChangeType = "network"
	ChangeTypeResources   ChangeType = "resources"
	ChangeTypeEnvironment ChangeType = "environment"
	ChangeTypeEntrypoint  ChangeType = "entrypoint"
)

type RiskLevel string

const (
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
)

type ContainerState struct {
	ID          string
	Image       string
	Config      ContainerConfig
	Filesystem  []FileEntry
	Environment map[string]string
	Processes   []ProcessInfo
}

type ContainerConfig struct {
	Entrypoint []string
	Cmd        []string
	WorkingDir string
	User       string
	Env        []string
	Volumes    []string
	Ports      []PortMapping
}

type FileEntry struct {
	Path string
	Size int64
	Mode uint32
}

type ProcessInfo struct {
	PID     int
	Command string
}

type PortMapping struct {
	ContainerPort int
	HostPort      int
	Protocol      string
}
