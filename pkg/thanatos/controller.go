package thanatos

import (
	"context"
	"errors"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes/audit"
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// ShutdownController manages graceful termination lifecycle.
type ShutdownController struct {
	Runtime        tartarus.SandboxRuntime
	Hypnos         *hypnos.Manager
	PolicyResolver PolicyResolver
	Exporter       *Exporter
	Auditor        audit.Auditor
	Metrics        hermes.Metrics
	Logger         hermes.Logger
	now            func() time.Time
}

// ShutdownControllerConfig holds configuration for the controller.
type ShutdownControllerConfig struct {
	Runtime        tartarus.SandboxRuntime
	Hypnos         *hypnos.Manager
	PolicyResolver PolicyResolver
	Exporter       *Exporter
	Auditor        audit.Auditor
	Metrics        hermes.Metrics
	Logger         hermes.Logger
}

// NewShutdownController creates a new ShutdownController.
func NewShutdownController(cfg ShutdownControllerConfig) *ShutdownController {
	if cfg.PolicyResolver == nil {
		cfg.PolicyResolver = NewStaticPolicyResolver(nil)
	}
	return &ShutdownController{
		Runtime:        cfg.Runtime,
		Hypnos:         cfg.Hypnos,
		PolicyResolver: cfg.PolicyResolver,
		Exporter:       cfg.Exporter,
		Auditor:        cfg.Auditor,
		Metrics:        cfg.Metrics,
		Logger:         cfg.Logger,
		now:            time.Now,
	}
}

// TerminationRequest specifies termination parameters.
type TerminationRequest struct {
	SandboxID      domain.SandboxID
	TemplateID     domain.TemplateID
	Reason         TerminationReason
	RequestedBy    *audit.Identity // Who initiated the termination
	ForceTimeout   time.Duration   // Override policy grace (0 = use policy)
	SkipExport     bool            // Skip logs/artifacts export
	SkipCheckpoint bool            // Skip checkpoint even if policy requires it
}

// ControllerResult captures the outcome of a termination attempt.
type ControllerResult struct {
	SandboxID    domain.SandboxID  `json:"sandbox_id"`
	Phase        Phase             `json:"phase"`
	Reason       TerminationReason `json:"reason"`
	Policy       *GracePolicy      `json:"policy,omitempty"`
	GraceUsed    time.Duration     `json:"grace_used"`
	ExitCode     *int              `json:"exit_code,omitempty"`
	Checkpoint   string            `json:"checkpoint,omitempty"`
	ExportResult *ExportResult     `json:"export_result,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
	InitiatedAt  time.Time         `json:"initiated_at"`
	CompletedAt  time.Time         `json:"completed_at"`
}

// RequestTermination initiates graceful shutdown with policy resolution.
func (c *ShutdownController) RequestTermination(ctx context.Context, req *TerminationRequest) (*ControllerResult, error) {
	if req == nil {
		return nil, errors.New("termination request cannot be nil")
	}

	start := c.now()
	result := &ControllerResult{
		SandboxID:   req.SandboxID,
		Phase:       PhaseInitiated,
		Reason:      req.Reason,
		InitiatedAt: start,
	}

	// Emit metric for termination initiation
	c.recordMetric("thanatos_controller_terminate_total", 1, "reason", req.Reason.String())

	// 1. Resolve policy
	policy, err := c.PolicyResolver.ResolvePolicy(ctx, req.SandboxID, req.TemplateID, req.Reason)
	if err != nil {
		c.log(ctx, "error", "Failed to resolve policy", map[string]any{
			"sandbox_id": req.SandboxID,
			"error":      err.Error(),
		})
		// Use default policy on error
		policy = &GracePolicy{
			ID:           "fallback",
			Name:         "Fallback Policy",
			DefaultGrace: DefaultGracePeriod,
		}
	}
	result.Policy = policy

	// Record audit event - initiated
	c.recordAuditEvent(ctx, req, result, "initiated", nil)

	// 2. Export logs/artifacts if configured
	if !req.SkipExport && c.Exporter != nil && (policy.ExportLogs || policy.ExportArtifacts) {
		c.log(ctx, "info", "Exporting data before termination", map[string]any{
			"sandbox_id":       req.SandboxID,
			"export_logs":      policy.ExportLogs,
			"export_artifacts": policy.ExportArtifacts,
		})

		exportResult, exportErr := c.Exporter.ExportForTermination(ctx, req.SandboxID, policy)
		result.ExportResult = exportResult
		if exportErr != nil {
			c.log(ctx, "warn", "Export failed, continuing with termination", map[string]any{
				"sandbox_id": req.SandboxID,
				"error":      exportErr.Error(),
			})
			c.recordMetric("thanatos_export_failed_total", 1)
		} else {
			c.recordMetric("thanatos_export_success_total", 1)
		}
	}

	// 3. Create checkpoint if configured
	if !req.SkipCheckpoint && policy.CheckpointFirst && c.Hypnos != nil {
		c.log(ctx, "info", "Creating checkpoint before termination", map[string]any{
			"sandbox_id": req.SandboxID,
		})

		rec, err := c.Hypnos.Sleep(ctx, req.SandboxID, &hypnos.SleepOptions{GracefulShutdown: true})
		if err != nil {
			c.log(ctx, "warn", "Checkpoint failed, falling back to graceful shutdown", map[string]any{
				"sandbox_id": req.SandboxID,
				"error":      err.Error(),
			})
			c.recordMetric("thanatos_checkpoint_failed_total", 1)
			// Fall through to graceful shutdown
		} else {
			result.Phase = PhaseCheckpointed
			result.Checkpoint = rec.SnapshotKey
			result.CompletedAt = c.now()
			result.GraceUsed = result.CompletedAt.Sub(start)
			c.recordMetric("thanatos_checkpoint_success_total", 1)
			c.recordAuditEvent(ctx, req, result, "checkpointed", nil)
			return result, nil
		}
	}

	// 4. Graceful shutdown with grace period
	grace := policy.EffectiveGrace(req.ForceTimeout)
	c.log(ctx, "info", "Starting graceful shutdown", map[string]any{
		"sandbox_id":   req.SandboxID,
		"grace_period": grace.String(),
	})

	if err := c.Runtime.Shutdown(ctx, req.SandboxID); err != nil {
		result.Phase = PhaseFailed
		result.ErrorMessage = err.Error()
		result.CompletedAt = c.now()
		result.GraceUsed = result.CompletedAt.Sub(start)
		c.recordMetric("thanatos_shutdown_failed_total", 1)
		c.recordAuditEvent(ctx, req, result, "failed", err)
		return result, err
	}
	result.Phase = PhaseGraceful

	// 5. Wait with grace period
	waitCtx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()

	if err := c.Runtime.Wait(waitCtx, req.SandboxID); err != nil {
		if waitCtx.Err() == context.DeadlineExceeded {
			// Grace period exceeded, force kill
			c.log(ctx, "warn", "Grace period exceeded, forcing kill", map[string]any{
				"sandbox_id":   req.SandboxID,
				"grace_period": grace.String(),
			})
			_ = c.Runtime.Kill(context.Background(), req.SandboxID)
			result.Phase = PhaseKilled
			result.ErrorMessage = "grace period exceeded; sandbox killed"
			result.CompletedAt = c.now()
			result.GraceUsed = result.CompletedAt.Sub(start)
			c.recordMetric("thanatos_grace_timeout_total", 1)
			c.recordAuditEvent(ctx, req, result, "killed", errors.New(result.ErrorMessage))
			return result, errors.New(result.ErrorMessage)
		}
		result.Phase = PhaseFailed
		result.ErrorMessage = err.Error()
		result.CompletedAt = c.now()
		result.GraceUsed = result.CompletedAt.Sub(start)
		c.recordMetric("thanatos_wait_failed_total", 1)
		c.recordAuditEvent(ctx, req, result, "failed", err)
		return result, err
	}

	// 6. Graceful shutdown succeeded
	if run, err := c.Runtime.Inspect(ctx, req.SandboxID); err == nil {
		result.ExitCode = run.ExitCode
	}
	result.Phase = PhaseCompleted
	result.CompletedAt = c.now()
	result.GraceUsed = result.CompletedAt.Sub(start)

	c.recordMetric("thanatos_graceful_success_total", 1)
	c.recordMetric("thanatos_phase_duration_seconds", result.GraceUsed.Seconds(), "phase", string(PhaseCompleted))
	c.recordAuditEvent(ctx, req, result, "completed", nil)

	return result, nil
}

// recordMetric records a metric if metrics are configured.
func (c *ShutdownController) recordMetric(name string, value float64, labelKV ...string) {
	if c.Metrics == nil {
		return
	}

	labels := make([]hermes.Label, 0, len(labelKV)/2)
	for i := 0; i+1 < len(labelKV); i += 2 {
		labels = append(labels, hermes.Label{Key: labelKV[i], Value: labelKV[i+1]})
	}

	if len(labels) > 0 {
		c.Metrics.IncCounter(name, value, labels...)
	} else {
		c.Metrics.IncCounter(name, value)
	}
}

// log logs a message if logger is configured.
func (c *ShutdownController) log(ctx context.Context, level, msg string, fields map[string]any) {
	if c.Logger == nil {
		return
	}

	switch level {
	case "error":
		c.Logger.Error(ctx, msg, fields)
	case "warn":
		c.Logger.Info(ctx, msg, fields)
	default:
		c.Logger.Info(ctx, msg, fields)
	}
}

// recordAuditEvent records an audit event if auditor is configured.
func (c *ShutdownController) recordAuditEvent(ctx context.Context, req *TerminationRequest, result *ControllerResult, status string, err error) {
	if c.Auditor == nil {
		return
	}

	auditResult := audit.ResultSuccess
	if err != nil {
		auditResult = audit.ResultError
	}

	event := &audit.Event{
		Timestamp: c.now(),
		Action:    audit.ActionTerminate,
		Result:    auditResult,
		Resource: audit.Resource{
			Type: "sandbox",
			ID:   string(req.SandboxID),
		},
		Identity: req.RequestedBy,
		Metadata: map[string]interface{}{
			"reason":     req.Reason.String(),
			"phase":      string(result.Phase),
			"status":     status,
			"grace_used": result.GraceUsed.String(),
		},
	}

	if result.Checkpoint != "" {
		event.Metadata["checkpoint"] = result.Checkpoint
	}
	if result.ExportResult != nil && result.ExportResult.LogsKey != "" {
		event.Metadata["logs_key"] = result.ExportResult.LogsKey
	}
	if err != nil {
		event.ErrorMessage = err.Error()
	}

	if auditErr := c.Auditor.Record(ctx, event); auditErr != nil {
		c.log(ctx, "error", "Failed to record audit event", map[string]any{
			"sandbox_id": req.SandboxID,
			"error":      auditErr.Error(),
		})
	}
}

// GracefulKillHandler creates a handler function for Erinyes integration.
// Returns a function that attempts graceful shutdown and returns true if successful.
func (c *ShutdownController) GracefulKillHandler(templateID domain.TemplateID) func(ctx context.Context, id domain.SandboxID, reason string) bool {
	return func(ctx context.Context, id domain.SandboxID, reason string) bool {
		termReason := mapToTerminationReason(reason)

		req := &TerminationRequest{
			SandboxID:  id,
			TemplateID: templateID,
			Reason:     termReason,
		}

		result, err := c.RequestTermination(ctx, req)
		if err != nil {
			return false
		}

		return result.Phase == PhaseCompleted || result.Phase == PhaseCheckpointed
	}
}

// mapToTerminationReason maps enforcement reason strings to TerminationReason.
func mapToTerminationReason(reason string) TerminationReason {
	switch reason {
	case "runtime_exceeded", "time_limit":
		return ReasonTimeLimit
	case "memory_exceeded", "resource_limit":
		return ReasonResourceLimit
	case "network_egress_exceeded", "network_ingress_exceeded", "banned_ip_attempts_exceeded":
		return ReasonNetworkViolation
	case "policy_breach":
		return ReasonPolicyBreach
	default:
		return ReasonPolicyBreach
	}
}
