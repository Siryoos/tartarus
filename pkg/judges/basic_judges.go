package judges

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

// ResourceJudge validates resource requests against policy limits.
type ResourceJudge struct {
	policyRepo themis.Repository
	logger     hermes.Logger
}

// NewResourceJudge creates a new resource judge.
func NewResourceJudge(policyRepo themis.Repository, logger hermes.Logger) *ResourceJudge {
	return &ResourceJudge{
		policyRepo: policyRepo,
		logger:     logger,
	}
}

// PreAdmit validates a sandbox request's resource requirements against policy.
func (j *ResourceJudge) PreAdmit(ctx context.Context, req *domain.SandboxRequest) (Verdict, error) {
	// Load policy for the request's template
	policy, err := j.policyRepo.GetPolicy(ctx, req.Template)
	if err != nil {
		j.logger.Error(ctx, "Failed to load policy for resource validation", map[string]any{
			"template": req.Template,
			"error":    err,
		})
		return VerdictReject, fmt.Errorf("failed to load policy: %w", err)
	}

	// Validate CPU
	if req.Resources.CPU > policy.Resources.CPU {
		j.logger.Info(ctx, "Request rejected: CPU exceeds policy limit", map[string]any{
			"sandbox_id":    req.ID,
			"template":      req.Template,
			"requested_cpu": req.Resources.CPU,
			"policy_cpu":    policy.Resources.CPU,
		})
		return VerdictReject, nil
	}

	// Validate Memory
	if req.Resources.Mem > policy.Resources.Mem {
		j.logger.Info(ctx, "Request rejected: Memory exceeds policy limit", map[string]any{
			"sandbox_id":    req.ID,
			"template":      req.Template,
			"requested_mem": req.Resources.Mem,
			"policy_mem":    policy.Resources.Mem,
		})
		return VerdictReject, nil
	}

	j.logger.Info(ctx, "Request passed resource validation", map[string]any{
		"sandbox_id": req.ID,
		"template":   req.Template,
		"cpu":        req.Resources.CPU,
		"mem":        req.Resources.Mem,
	})

	return VerdictAccept, nil
}

// NetworkJudge validates network policies against deny-list.
type NetworkJudge struct {
	denyList []netip.Prefix
	logger   hermes.Logger
}

// NewNetworkJudge creates a new network judge.
func NewNetworkJudge(denyList []netip.Prefix, logger hermes.Logger) *NetworkJudge {
	return &NetworkJudge{
		denyList: denyList,
		logger:   logger,
	}
}

// PreAdmit validates a sandbox request's network policy.
func (j *NetworkJudge) PreAdmit(ctx context.Context, req *domain.SandboxRequest) (Verdict, error) {
	networkID := req.NetworkRef.ID
	networkName := req.NetworkRef.Name

	// Accept secure defaults (lockdown policies)
	if strings.Contains(strings.ToLower(networkID), "no-net") ||
		strings.Contains(strings.ToLower(networkID), "lockdown") ||
		strings.Contains(strings.ToLower(networkName), "no internet") {
		j.logger.Info(ctx, "Request passed network validation: secure default", map[string]any{
			"sandbox_id": req.ID,
			"network_id": networkID,
		})
		return VerdictAccept, nil
	}

	// For now, reject all other network policies (conservative approach)
	// Future enhancement: implement full CIDR-based validation
	j.logger.Info(ctx, "Request rejected: network policy not in allowed list", map[string]any{
		"sandbox_id":   req.ID,
		"network_id":   networkID,
		"network_name": networkName,
	})
	return VerdictReject, nil
}
