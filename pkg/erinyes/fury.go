package erinyes

import (
	"context"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// PolicySnapshot is a runtime view of enforcement parameters.

type PolicySnapshot struct {
	MaxRuntime             time.Duration
	MaxCPU                 domain.MilliCPU
	MaxMemory              domain.Megabytes
	MaxNetworkEgressBytes  int64
	MaxNetworkIngressBytes int64
	MaxBannedIPAttempts    int
	KillOnBreach           bool
}

// Fury watches a running sandbox and enforces runtime policy.

type Fury interface {
	// Arm starts watchers for a given run ID.
	Arm(ctx context.Context, run *domain.SandboxRun, policy *PolicySnapshot) error

	// Disarm stops watchers (run completed normally).
	Disarm(ctx context.Context, runID domain.SandboxID) error
}
