package olympus

import (
	"context"
	"fmt"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/persephone"
)

// Scaler manages predictive scaling and pre-warming
type Scaler struct {
	Persephone        persephone.SeasonalScaler
	Hades             hades.Registry
	Manager           *Manager
	Logger            hermes.Logger
	Metrics           hermes.Metrics
	seasonActivator   *persephone.SeasonActivator
	capacityOptimizer *persephone.CapacityOptimizer
}

func NewScaler(p persephone.SeasonalScaler, h hades.Registry, m *Manager, l hermes.Logger, met hermes.Metrics) *Scaler {
	// Initialize season activator
	scheduler, _ := persephone.NewCronScheduler("UTC")
	activator := persephone.NewSeasonActivator(scheduler)

	return &Scaler{
		Persephone:        p,
		Hades:             h,
		Manager:           m,
		Logger:            l,
		Metrics:           met,
		seasonActivator:   activator,
		capacityOptimizer: persephone.NewCapacityOptimizer(),
	}
}

// RegisterSeason adds a season for automatic activation
func (s *Scaler) RegisterSeason(season *persephone.Season) {
	if s.seasonActivator != nil {
		s.seasonActivator.RegisterSeason(season)
	}
}

// Run starts the scaling loop
func (s *Scaler) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	s.Logger.Info(ctx, "Starting Persephone Scaler", nil)

	for {
		select {
		case <-ctx.Done():
			s.Logger.Info(ctx, "Stopping Persephone Scaler", nil)
			return
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.Logger.Error(ctx, "Scaler tick failed", map[string]any{"error": err})
			}
		}
	}
}

func (s *Scaler) tick(ctx context.Context) error {
	// 1. Gather Metrics
	runs, err := s.Hades.ListRuns(ctx)
	if err != nil {
		return fmt.Errorf("failed to list runs: %w", err)
	}

	activeCount := 0
	launchCount := 0
	errorCount := 0
	for _, run := range runs {
		switch run.Status {
		case domain.RunStatusRunning, domain.RunStatusScheduled:
			activeCount++
		case domain.RunStatusPending:
			launchCount++ // Jobs waiting to be launched
		case domain.RunStatusFailed:
			errorCount++
		}
	}

	// Gather CPU/Memory utilization from node heartbeats
	var totalCPUUtil, totalMemUtil float64
	var nodeCount int
	nodes, err := s.Hades.ListNodes(ctx)
	if err == nil && len(nodes) > 0 {
		for _, node := range nodes {
			// Calculate utilization as allocated/capacity
			if node.Capacity.CPU > 0 {
				cpuUtil := float64(node.Allocated.CPU) / float64(node.Capacity.CPU)
				totalCPUUtil += cpuUtil
			}
			if node.Capacity.Mem > 0 {
				memUtil := float64(node.Allocated.Mem) / float64(node.Capacity.Mem)
				totalMemUtil += memUtil
			}
			nodeCount++
		}
		if nodeCount > 0 {
			totalCPUUtil /= float64(nodeCount)
			totalMemUtil /= float64(nodeCount)
		}
	}

	// Get queue depth
	queueDepth := 0
	if s.Manager != nil && s.Manager.Queue != nil {
		queueDepth = s.Manager.Queue.Len(ctx)
	}

	// 2. Learn with comprehensive metrics
	record := &persephone.UsageRecord{
		Timestamp:   time.Now(),
		ActiveVMs:   activeCount,
		QueueDepth:  queueDepth,
		CPUUtil:     totalCPUUtil,
		MemoryUtil:  totalMemUtil,
		LaunchCount: launchCount,
		ErrorCount:  errorCount,
	}

	// Emit metrics for observability
	s.Metrics.SetGauge("scaler_active_vms", float64(activeCount))
	s.Metrics.SetGauge("scaler_queue_depth", float64(queueDepth))
	s.Metrics.SetGauge("scaler_cpu_utilization", totalCPUUtil)
	s.Metrics.SetGauge("scaler_memory_utilization", totalMemUtil)
	s.Metrics.SetGauge("scaler_pending_launches", float64(launchCount))
	s.Metrics.SetGauge("scaler_error_count", float64(errorCount))

	if err := s.Persephone.Learn(ctx, []*persephone.UsageRecord{record}); err != nil {
		s.Logger.Error(ctx, "Failed to update Persephone model", map[string]any{"error": err})
	}

	// 3. Auto Season Activation
	if s.seasonActivator != nil {
		season, err := s.seasonActivator.EvaluateSeasons(ctx, time.Now())
		if err != nil {
			s.Logger.Error(ctx, "Failed to evaluate seasons", map[string]any{"error": err})
		} else if season != nil {
			currentSeason, _ := s.Persephone.CurrentSeason(ctx)
			if currentSeason == nil || currentSeason.ID != season.ID {
				s.Logger.Info(ctx, "Auto-activating season", map[string]any{"season": season.Name})
				if err := s.Persephone.ApplySeason(ctx, season.ID); err != nil {
					s.Logger.Error(ctx, "Failed to apply season", map[string]any{"error": err})
				} else {
					s.Metrics.IncCounter("persephone_season_transitions_total", 1,
						hermes.Label{Key: "season", Value: season.ID})
				}
			}
			// Emit active season metric
			s.Metrics.SetGauge("persephone_season_active", 1,
				hermes.Label{Key: "season_id", Value: season.ID},
				hermes.Label{Key: "season_name", Value: season.Name})
		}
	}

	// 4. Get Current Season
	season, err := s.Persephone.CurrentSeason(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current season: %w", err)
	}
	if season == nil {
		// No active season, nothing to do
		return nil
	}

	// 5. Emit capacity recommendation metrics
	if s.capacityOptimizer != nil {
		// Get historical records for recommendation
		// This is a simplified version - in production, fetch from Persephone history
		recommendation, err := s.Persephone.RecommendCapacity(ctx, season.TargetUtilization)
		if err == nil && recommendation != nil {
			s.Metrics.SetGauge("persephone_capacity_recommendation", float64(recommendation.RecommendedNodes),
				hermes.Label{Key: "reason", Value: recommendation.Reason})
			s.Metrics.SetGauge("persephone_capacity_current", float64(recommendation.CurrentNodes))
		}
	}

	// 6. Pre-warming Logic
	if season.Prewarming.PoolSize > 0 && len(season.Prewarming.Templates) > 0 {
		for _, tplID := range season.Prewarming.Templates {
			if err := s.ensureWarmPool(ctx, domain.TemplateID(tplID), season.Prewarming.PoolSize); err != nil {
				s.Logger.Error(ctx, "Failed to ensure warm pool", map[string]any{
					"template": tplID,
					"error":    err,
				})
			}
		}
	}

	return nil
}

func (s *Scaler) ensureWarmPool(ctx context.Context, tplID domain.TemplateID, targetSize int) error {
	// Count existing warm sandboxes for this template
	runs, err := s.Hades.ListRuns(ctx)
	if err != nil {
		return err
	}

	warmCount := 0
	for _, run := range runs {
		if run.Template == tplID && run.Status == domain.RunStatusRunning {
			if val, ok := run.Metadata["warm"]; ok && val == "true" {
				warmCount++
			}
		}
	}

	// If we have enough, do nothing
	if warmCount >= targetSize {
		return nil
	}

	// Calculate how many to create
	needed := targetSize - warmCount
	s.Logger.Info(ctx, "Pre-warming sandboxes", map[string]any{
		"template": tplID,
		"needed":   needed,
		"current":  warmCount,
		"target":   targetSize,
	})

	for i := 0; i < needed; i++ {
		// Create a warm sandbox request
		// We need to get the template spec to know resources
		tpl, err := s.Manager.Templates.GetTemplate(ctx, tplID)
		if err != nil {
			s.Logger.Error(ctx, "Failed to get template for pre-warming", map[string]any{"template": tplID, "error": err})
			continue
		}

		req := &domain.SandboxRequest{
			Template:  tplID,
			Command:   tpl.WarmupCommand, // Use warmup command if available, else default?
			Resources: tpl.Resources,
			Metadata: map[string]string{
				"warm": "true",
				"type": "prewarm",
			},
			NetworkRef: domain.NetworkPolicyRef{
				ID: "lockdown-no-net", // Default to no net for warm pool for safety
			},
		}

		// If no warmup command, use a sleep or no-op to keep it running?
		// If we submit with empty command, it might fail or exit immediately depending on image.
		// Let's assume the template has a default command or we use a long sleep.
		if len(req.Command) == 0 {
			req.Command = []string{"/bin/sh", "-c", "sleep 3600"} // Sleep for an hour?
			// Ideally we want it to stay up until claimed.
			// But for this MVP, sleep is fine.
		}

		if err := s.Manager.Submit(ctx, req); err != nil {
			s.Logger.Error(ctx, "Failed to submit pre-warm request", map[string]any{"error": err})
		}
	}

	return nil
}
