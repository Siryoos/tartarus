package plugins

import (
	"context"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// PluginType identifies the category of plugin.
type PluginType string

const (
	PluginTypeJudge PluginType = "judge"
	PluginTypeFury  PluginType = "fury"
)

// Verdict mirrors judges.Verdict for plugin isolation.
type Verdict int

const (
	VerdictAccept Verdict = iota
	VerdictReject
	VerdictQuarantine
)

// Classification mirrors judges.Classification for plugin isolation.
type Classification struct {
	Verdict Verdict           `json:"verdict"`
	Reason  string            `json:"reason"`
	Labels  map[string]string `json:"labels"`
}

// Plugin is the base interface all plugins must implement.
type Plugin interface {
	// Name returns the unique identifier for this plugin.
	Name() string

	// Version returns the semantic version string.
	Version() string

	// Type returns the plugin category (judge or fury).
	Type() PluginType

	// Init is called once when the plugin is loaded.
	Init(config map[string]any) error

	// Close is called when the plugin is unloaded.
	Close() error
}

// JudgePlugin extends Plugin for admission and post-execution evaluation.
type JudgePlugin interface {
	Plugin

	// PreAdmit is called before sandbox scheduling.
	// Return VerdictAccept to allow, VerdictReject to deny.
	PreAdmit(ctx context.Context, req *domain.SandboxRequest) (Verdict, error)

	// PostHoc is called after sandbox completion for classification.
	PostHoc(ctx context.Context, run *domain.SandboxRun) (*Classification, error)
}

// PolicySnapshot is a simplified view of runtime enforcement parameters for plugins.
type PolicySnapshot struct {
	MaxRuntimeSeconds      int64
	MaxCPUMillis           int64
	MaxMemoryMB            int64
	MaxNetworkEgressBytes  int64
	MaxNetworkIngressBytes int64
	MaxBannedIPAttempts    int
	KillOnBreach           bool
}

// FuryPlugin extends Plugin for runtime enforcement.
type FuryPlugin interface {
	Plugin

	// Arm starts enforcement watchers for a running sandbox.
	Arm(ctx context.Context, run *domain.SandboxRun, policy *PolicySnapshot) error

	// Disarm stops enforcement watchers when sandbox completes normally.
	Disarm(ctx context.Context, runID domain.SandboxID) error
}

// PluginSymbol is the symbol name that plugins must export.
// The exported variable must implement Plugin interface.
const PluginSymbol = "TartarusPlugin"
