package kampe

import (
	"context"
	"fmt"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// MigrationManager orchestrates the migration from a legacy runtime to a target runtime (microVM)
type MigrationManager struct {
	Source LegacyRuntime
	Target tartarus.SandboxRuntime
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(source LegacyRuntime, target tartarus.SandboxRuntime) *MigrationManager {
	return &MigrationManager{
		Source: source,
		Target: target,
	}
}

// MigrationResult captures the outcome of a migration attempt
type MigrationResult struct {
	OriginalID     string
	NewID          domain.SandboxID
	Duration       time.Duration
	Plan           *MigrationPlan
	Error          error
	RollbackStatus string // "none", "success", "failed"
}

// Migrate performs a cold migration of a container to a microVM
func (m *MigrationManager) Migrate(ctx context.Context, containerID string) *MigrationResult {
	start := time.Now()
	result := &MigrationResult{
		OriginalID: containerID,
	}

	// 1. Assess
	plan, err := m.Source.MigrateToMicroVM(ctx, containerID)
	if err != nil {
		result.Error = fmt.Errorf("migration assessment failed: %w", err)
		result.Duration = time.Since(start)
		return result
	}
	result.Plan = plan

	if plan.RiskLevel == RiskLevelHigh {
		result.Error = fmt.Errorf("migration risk too high: %v", plan.RequiredChanges)
		result.Duration = time.Since(start)
		return result
	}

	// 2. Pause Source
	if err := m.Source.Pause(ctx, domain.SandboxID(containerID)); err != nil {
		result.Error = fmt.Errorf("failed to pause source container: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Deferred rollback: resume source if migration fails
	defer func() {
		if result.Error != nil {
			resumeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if resumeErr := m.Source.Resume(resumeCtx, domain.SandboxID(containerID)); resumeErr != nil {
				result.RollbackStatus = "failed"
				// augment error
				result.Error = fmt.Errorf("%v; rollback failed: %v", result.Error, resumeErr)
			} else {
				result.RollbackStatus = "success"
			}
		} else {
			result.RollbackStatus = "none"
		}
		result.Duration = time.Since(start)
	}()

	// 3. Export State
	state, err := m.Source.ExportState(ctx, containerID)
	if err != nil {
		result.Error = fmt.Errorf("failed to export state: %w", err)
		return result
	}

	// 4. Translate State to SandboxRequest
	req, cfg := m.translateState(state)

	// 5. Launch Target
	run, err := m.Target.Launch(ctx, req, cfg)
	if err != nil {
		result.Error = fmt.Errorf("failed to launch target microVM: %w", err)
		return result
	}
	result.NewID = run.ID

	// 6. Verify (Wait for running state)
	// Launch usually returns when VM is started, but we double check
	runState, err := m.Target.Inspect(ctx, run.ID)
	if err != nil {
		// Attempt cleanup of new VM
		_ = m.Target.Kill(context.Background(), run.ID)
		result.Error = fmt.Errorf("failed to verify new VM: %w", err)
		return result
	}
	if runState.Status != domain.RunStatusRunning {
		_ = m.Target.Kill(context.Background(), run.ID)
		result.Error = fmt.Errorf("new VM not running, status: %s", runState.Status)
		return result
	}

	// 7. Cutover (Kill old container)
	// We do this in a separate context to ensure it runs even if parent context is expiring
	killCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.Source.Kill(killCtx, domain.SandboxID(containerID)); err != nil {
		// This is a partial success: migration worked, but old cleanup failed
		// We log it but consider migration successful
		// In a real system we might flag this for manual intervention
		fmt.Printf("Warning: failed to kill old container %s: %v\n", containerID, err)
	}

	return result
}

// translateState converts container state to Tartarus domain types
func (m *MigrationManager) translateState(state *ContainerState) (*domain.SandboxRequest, tartarus.VMConfig) {
	// Construct SandboxRequest
	req := &domain.SandboxRequest{
		ID:       domain.SandboxID(fmt.Sprintf("%s-vm-%d", state.ID, time.Now().Unix())),
		Template: domain.TemplateID(state.Image), // This might need mapping logic in production
		Command:  state.Config.Cmd,
		// Args: state.Config.Args, // ContainerConfig often mixes Entrypoint/Cmd
		Env: state.Environment,
		Resources: domain.ResourceSpec{
			// Defaults or derived from limits if available
			// Since we don't have resource limits in ContainerState yet (it was missing in definition),
			// we use conservative defaults suitable for a microVM
			CPU: 1000,
			Mem: 512,
		},
	}

	// Handle Entrypoint vs Cmd:
	// Docker: Entrypoint + Cmd
	// Tartarus: Command + Args (roughly)
	// Ideally we concatenate them or use Command for Entrypoint and Args for Cmd
	if len(state.Config.Entrypoint) > 0 {
		req.Command = state.Config.Entrypoint
		req.Args = state.Config.Cmd
	} else {
		req.Command = state.Config.Cmd
	}

	// Construct VMConfig
	cfg := tartarus.VMConfig{
		CPUs:     1,
		MemoryMB: 512,
		// Network config would normally be allocated by IPAM here
	}

	return req, cfg
}
