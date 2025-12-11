package plugins

import (
	"context"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
)

// JudgePluginAdapter wraps a JudgePlugin to implement judges.PreJudge and judges.PostJudge.
type JudgePluginAdapter struct {
	plugin JudgePlugin
}

// NewJudgePluginAdapter creates a new adapter for a judge plugin.
func NewJudgePluginAdapter(plugin JudgePlugin) *JudgePluginAdapter {
	return &JudgePluginAdapter{plugin: plugin}
}

// PreAdmit implements judges.PreJudge.
func (a *JudgePluginAdapter) PreAdmit(ctx context.Context, req *domain.SandboxRequest) (judges.Verdict, error) {
	verdict, err := a.plugin.PreAdmit(ctx, req)
	if err != nil {
		return judges.VerdictReject, err
	}
	return convertVerdict(verdict), nil
}

// PostHoc implements judges.PostJudge.
func (a *JudgePluginAdapter) PostHoc(ctx context.Context, run *domain.SandboxRun) (*judges.Classification, error) {
	classification, err := a.plugin.PostHoc(ctx, run)
	if err != nil {
		return nil, err
	}
	if classification == nil {
		return nil, nil
	}
	return &judges.Classification{
		Verdict: convertVerdict(classification.Verdict),
		Reason:  classification.Reason,
		Labels:  classification.Labels,
	}, nil
}

// Name returns the plugin name.
func (a *JudgePluginAdapter) Name() string {
	return a.plugin.Name()
}

// convertVerdict converts plugin Verdict to judges.Verdict.
func convertVerdict(v Verdict) judges.Verdict {
	switch v {
	case VerdictAccept:
		return judges.VerdictAccept
	case VerdictReject:
		return judges.VerdictReject
	case VerdictQuarantine:
		return judges.VerdictQuarantine
	default:
		return judges.VerdictReject
	}
}

// WrapJudgePlugins wraps multiple judge plugins as PreJudge/PostJudge.
func WrapJudgePlugins(plugins []JudgePlugin) ([]judges.PreJudge, []judges.PostJudge) {
	var pre []judges.PreJudge
	var post []judges.PostJudge

	for _, p := range plugins {
		adapter := NewJudgePluginAdapter(p)
		pre = append(pre, adapter)
		post = append(post, adapter)
	}

	return pre, post
}
