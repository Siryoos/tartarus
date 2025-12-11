//go:build ignore
// +build ignore

// This is an example plugin source file.
// Build with: go build -buildmode=plugin -o rate-limit-judge.so main.go
//
// Note: Go plugins only work on Linux with matching Go versions.

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/plugins"
)

// RateLimitJudge enforces per-tenant request rate limits.
type RateLimitJudge struct {
	maxRequests    int
	burstSize      int
	lookbackWindow time.Duration

	mu       sync.Mutex
	requests map[string][]time.Time // tenant -> request timestamps
}

func (r *RateLimitJudge) Name() string {
	return "rate-limit-judge"
}

func (r *RateLimitJudge) Version() string {
	return "1.0.0"
}

func (r *RateLimitJudge) Type() plugins.PluginType {
	return plugins.PluginTypeJudge
}

func (r *RateLimitJudge) Init(config map[string]any) error {
	r.requests = make(map[string][]time.Time)

	// Parse config
	if v, ok := config["maxRequestsPerMinute"].(int); ok {
		r.maxRequests = v
	} else {
		r.maxRequests = 60
	}

	if v, ok := config["burstSize"].(int); ok {
		r.burstSize = v
	} else {
		r.burstSize = 10
	}

	if v, ok := config["lookbackWindow"].(string); ok {
		d, err := time.ParseDuration(v)
		if err == nil {
			r.lookbackWindow = d
		}
	}
	if r.lookbackWindow == 0 {
		r.lookbackWindow = time.Minute
	}

	return nil
}

func (r *RateLimitJudge) Close() error {
	return nil
}

func (r *RateLimitJudge) PreAdmit(ctx context.Context, req *domain.SandboxRequest) (plugins.Verdict, error) {
	tenant := req.TenantID
	if tenant == "" {
		tenant = "default"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.lookbackWindow)

	// Get recent requests for tenant
	recent := r.requests[tenant]

	// Filter to recent window
	var filtered []time.Time
	for _, t := range recent {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}

	// Check rate limit
	if len(filtered) >= r.maxRequests {
		return plugins.VerdictReject, fmt.Errorf("rate limit exceeded: %d requests in last %s", len(filtered), r.lookbackWindow)
	}

	// Check burst
	burstWindow := time.Second
	burstCutoff := now.Add(-burstWindow)
	var burstCount int
	for _, t := range filtered {
		if t.After(burstCutoff) {
			burstCount++
		}
	}
	if burstCount >= r.burstSize {
		return plugins.VerdictReject, fmt.Errorf("burst limit exceeded: %d requests in last second", burstCount)
	}

	// Record this request
	filtered = append(filtered, now)
	r.requests[tenant] = filtered

	return plugins.VerdictAccept, nil
}

func (r *RateLimitJudge) PostHoc(ctx context.Context, run *domain.SandboxRun) (*plugins.Classification, error) {
	// No post-hoc classification needed for rate limiting
	return nil, nil
}

// TartarusPlugin is the exported symbol that the plugin loader looks for.
var TartarusPlugin plugins.JudgePlugin = &RateLimitJudge{}
