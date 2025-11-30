package typhon

import (
	"context"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// Classifier determines if a sandbox should be quarantined
type Classifier interface {
	// ShouldQuarantine evaluates if a sandbox should be quarantined
	ShouldQuarantine(ctx context.Context, sandbox *domain.SandboxRequest) (bool, QuarantineReason, []Evidence)
}

// RuleBasedClassifier evaluates auto-quarantine triggers using CEL expressions
type RuleBasedClassifier struct {
	triggers []AutoQuarantineTrigger
	env      *cel.Env
}

// NewRuleBasedClassifier creates a classifier with the given triggers
func NewRuleBasedClassifier(triggers []AutoQuarantineTrigger) (*RuleBasedClassifier, error) {
	// Create CEL environment with sandbox context
	env, err := cel.NewEnv(
		cel.Variable("cpu", cel.IntType),
		cel.Variable("mem", cel.IntType),
		cel.Variable("template", cel.StringType),
		cel.Variable("metadata", cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable("network_egress", cel.IntType),
		cel.Variable("network_ingress", cel.IntType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	return &RuleBasedClassifier{
		triggers: triggers,
		env:      env,
	}, nil
}

// ShouldQuarantine evaluates all triggers against the sandbox
func (c *RuleBasedClassifier) ShouldQuarantine(ctx context.Context, sandbox *domain.SandboxRequest) (bool, QuarantineReason, []Evidence) {
	for _, trigger := range c.triggers {
		if !trigger.Enabled {
			continue
		}

		match, evidence := c.evaluateTrigger(sandbox, trigger)
		if match {
			return true, trigger.Reason, evidence
		}
	}

	return false, "", nil
}

// evaluateTrigger checks if a single trigger matches
func (c *RuleBasedClassifier) evaluateTrigger(sandbox *domain.SandboxRequest, trigger AutoQuarantineTrigger) (bool, []Evidence) {
	// Parse the CEL expression
	ast, issues := c.env.Compile(trigger.Condition)
	if issues != nil && issues.Err() != nil {
		// Invalid expression, skip this trigger
		return false, nil
	}

	// Create program
	prg, err := c.env.Program(ast)
	if err != nil {
		return false, nil
	}

	// Evaluate with sandbox context
	vars := map[string]interface{}{
		"cpu":      int64(sandbox.Resources.CPU),
		"mem":      int64(sandbox.Resources.Mem),
		"template": sandbox.Template,
		"metadata": sandbox.Metadata,
		// Network metrics would come from runtime monitoring
		// For now, use placeholders
		"network_egress":  int64(0),
		"network_ingress": int64(0),
	}

	result, _, err := prg.Eval(vars)
	if err != nil {
		return false, nil
	}

	// Check if result is true
	if boolResult, ok := result.Value().(bool); ok && boolResult {
		evidence := []Evidence{
			{
				Type:        EvidenceTypeSyscallTrace,
				Description: fmt.Sprintf("Auto-classification trigger: %s", trigger.Condition),
				Timestamp:   time.Now(),
			},
		}
		return true, evidence
	}

	return false, nil
}

// GetDefaultTriggers returns a set of common auto-quarantine triggers
func GetDefaultTriggers() []AutoQuarantineTrigger {
	return []AutoQuarantineTrigger{
		{
			Condition: `cpu > 8000`,
			Reason:    ReasonResourceAbuse,
			Enabled:   true,
		},
		{
			Condition: `mem > 16384`,
			Reason:    ReasonResourceAbuse,
			Enabled:   true,
		},
		{
			Condition: `metadata["untrusted"] == "true"`,
			Reason:    ReasonUntrustedSource,
			Enabled:   true,
		},
		{
			Condition: `network_egress > 1000000000`, // 1GB
			Reason:    ReasonNetworkAnomaly,
			Enabled:   true,
		},
		{
			Condition: `metadata["security_scan_failed"] == "true"`,
			Reason:    ReasonSecurityScan,
			Enabled:   true,
		},
	}
}

// NoopClassifier never quarantines
type NoopClassifier struct{}

func (n *NoopClassifier) ShouldQuarantine(ctx context.Context, sandbox *domain.SandboxRequest) (bool, QuarantineReason, []Evidence) {
	return false, "", nil
}
