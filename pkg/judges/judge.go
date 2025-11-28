package judges

import (
	"context"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type Verdict int

const (
	VerdictAccept Verdict = iota
	VerdictReject
	VerdictQuarantine
)

type Classification struct {
	Verdict Verdict           `json:"verdict"`
	Reason  string            `json:"reason"`
	Labels  map[string]string `json:"labels"`
}

// PreJudge runs before scheduling / execution.

type PreJudge interface {
	PreAdmit(ctx context.Context, req *domain.SandboxRequest) (Verdict, error)
}

// PostJudge runs after completion.

type PostJudge interface {
	PostHoc(ctx context.Context, run *domain.SandboxRun) (*Classification, error)
}

// Chain composes multiple judges.

type Chain struct {
	Pre  []PreJudge
	Post []PostJudge
}

func (c *Chain) RunPre(ctx context.Context, req *domain.SandboxRequest) (Verdict, error) {
	for _, j := range c.Pre {
		v, err := j.PreAdmit(ctx, req)
		if err != nil {
			return VerdictReject, err
		}
		if v != VerdictAccept {
			return v, nil
		}
	}
	return VerdictAccept, nil
}

func (c *Chain) RunPost(ctx context.Context, run *domain.SandboxRun) (*Classification, error) {
	out := &Classification{Verdict: VerdictAccept, Labels: map[string]string{}}
	for _, j := range c.Post {
		cl, err := j.PostHoc(ctx, run)
		if err != nil {
			return nil, err
		}
		if cl != nil {
			out.Verdict = cl.Verdict
			out.Reason += cl.Reason + "; "
			for k, v := range cl.Labels {
				out.Labels[k] = v
			}
		}
	}
	return out, nil
}
