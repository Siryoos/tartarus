package persephone

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// HibernationConfig defines when and how to hibernate sandboxes
type HibernationConfig struct {
	// Enabled controls whether hibernation is active for this season
	Enabled bool
	// IdleTimeout is the duration after which idle sandboxes are hibernated
	IdleTimeout time.Duration
	// ScheduledHibernation enables forced hibernation during off-peak hours
	ScheduledHibernation bool
	// HibernationStart is a cron expression for when scheduled hibernation begins
	HibernationStart string
	// HibernationEnd is a cron expression for when scheduled hibernation ends
	HibernationEnd string
	// MinWarmPool is the minimum number of sandboxes to keep warm during hibernation
	MinWarmPool int
	// WakeTriggers defines conditions that trigger wake from hibernation
	WakeTriggers []WakeTrigger
	// WakeLeadTime is how far ahead of scheduled end to start waking sandboxes
	WakeLeadTime time.Duration
}

// WakeTrigger defines conditions that trigger wake from hibernation
type WakeTrigger string

const (
	// WakeTriggerDemandSpike wakes sandboxes when demand exceeds capacity
	WakeTriggerDemandSpike WakeTrigger = "demand_spike"
	// WakeTriggerScheduled wakes sandboxes at scheduled times
	WakeTriggerScheduled WakeTrigger = "scheduled"
	// WakeTriggerManual requires explicit wake request
	WakeTriggerManual WakeTrigger = "manual"
	// WakeTriggerQueueDepth wakes based on job queue depth
	WakeTriggerQueueDepth WakeTrigger = "queue_depth"
)

// HypnosManager is the interface for Hypnos hibernation operations
type HypnosManager interface {
	// Sleep hibernates a sandbox
	Sleep(ctx context.Context, id domain.SandboxID, opts interface{}) (interface{}, error)
	// Wake restores a hibernating sandbox
	Wake(ctx context.Context, id domain.SandboxID) (interface{}, error)
	// List returns all sleeping sandboxes
	List() []interface{}
	// IsSleeping reports whether a sandbox is hibernating
	IsSleeping(id domain.SandboxID) bool
}

// SandboxLister provides information about active sandboxes
type SandboxLister interface {
	// ListActive returns active sandbox IDs with their last activity time
	ListActive(ctx context.Context) ([]SandboxActivity, error)
}

// SandboxActivity tracks sandbox activity state
type SandboxActivity struct {
	ID           domain.SandboxID
	LastActivity time.Time
	IsIdle       bool
}

// HibernationController manages sandbox hibernation cycles
type HibernationController struct {
	hypnos    HypnosManager
	lister    SandboxLister
	scaler    SeasonalScaler
	scheduler *CronScheduler
	metrics   hermes.Metrics

	mu            sync.RWMutex
	currentConfig *HibernationConfig
	running       bool
	stopCh        chan struct{}

	// Time source for testing
	now func() time.Time
}

// NewHibernationController creates a new hibernation controller
func NewHibernationController(
	hypnos HypnosManager,
	lister SandboxLister,
	scaler SeasonalScaler,
	scheduler *CronScheduler,
	metrics hermes.Metrics,
) *HibernationController {
	return &HibernationController{
		hypnos:    hypnos,
		lister:    lister,
		scaler:    scaler,
		scheduler: scheduler,
		metrics:   metrics,
		stopCh:    make(chan struct{}),
		now:       time.Now,
	}
}

// Start begins the hibernation control loop
func (c *HibernationController) Start(ctx context.Context, checkInterval time.Duration) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("hibernation controller already running")
	}
	c.running = true
	c.stopCh = make(chan struct{})
	c.mu.Unlock()

	if checkInterval <= 0 {
		checkInterval = 1 * time.Minute
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-c.stopCh:
			return nil
		case <-ticker.C:
			c.evaluate(ctx)
		}
	}
}

// Stop stops the hibernation control loop
func (c *HibernationController) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		close(c.stopCh)
		c.running = false
	}
}

// SetConfig updates the hibernation configuration
func (c *HibernationController) SetConfig(config *HibernationConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentConfig = config
}

// GetConfig returns the current hibernation configuration
func (c *HibernationController) GetConfig() *HibernationConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentConfig
}

func (c *HibernationController) evaluate(ctx context.Context) {
	config := c.GetConfig()
	if config == nil || !config.Enabled {
		return
	}

	now := c.now()

	// Check if we're in a scheduled hibernation window
	inHibernationWindow := c.IsInHibernationWindow(config, now)

	if inHibernationWindow {
		// Hibernate idle sandboxes, keeping min warm pool
		c.hibernateForSchedule(ctx, config)
	} else if config.IdleTimeout > 0 {
		// Outside hibernation window, still hibernate long-idle sandboxes
		c.hibernateIdle(ctx, config.IdleTimeout, 0) // No min pool outside schedule
	}

	// Check wake triggers
	c.checkWakeTriggers(ctx, config, now)
}

// EvaluateHibernation manually triggers hibernation evaluation for a season
func (c *HibernationController) EvaluateHibernation(ctx context.Context, season *Season) error {
	if season == nil || !season.Hibernation.Enabled {
		return nil
	}

	c.SetConfig(&season.Hibernation)
	c.evaluate(ctx)
	return nil
}

// HibernateIdle hibernates sandboxes that have been idle longer than the threshold
func (c *HibernationController) HibernateIdle(ctx context.Context, idleThreshold time.Duration) (int, error) {
	return c.hibernateIdle(ctx, idleThreshold, 0)
}

func (c *HibernationController) hibernateIdle(ctx context.Context, idleThreshold time.Duration, minWarm int) (int, error) {
	if c.lister == nil || c.hypnos == nil {
		return 0, nil
	}

	activities, err := c.lister.ListActive(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list active sandboxes: %w", err)
	}

	now := c.now()
	var hibernated int
	var warmCount int

	// Count non-idle as warm
	for _, a := range activities {
		if !a.IsIdle {
			warmCount++
		}
	}

	for _, activity := range activities {
		// Skip if we need to maintain min warm pool
		if warmCount <= minWarm {
			break
		}

		// Check if sandbox has been idle long enough
		idleDuration := now.Sub(activity.LastActivity)
		if activity.IsIdle && idleDuration >= idleThreshold {
			if _, err := c.hypnos.Sleep(ctx, activity.ID, nil); err != nil {
				// Log error but continue with other sandboxes
				if c.metrics != nil {
					c.metrics.IncCounter("persephone_hibernation_errors_total", 1,
						hermes.Label{Key: "operation", Value: "sleep"})
				}
				continue
			}

			hibernated++
			warmCount--

			if c.metrics != nil {
				c.metrics.IncCounter("persephone_hibernations_total", 1,
					hermes.Label{Key: "reason", Value: "idle"})
			}
		}
	}

	return hibernated, nil
}

func (c *HibernationController) hibernateForSchedule(ctx context.Context, config *HibernationConfig) {
	if c.lister == nil || c.hypnos == nil {
		return
	}

	activities, err := c.lister.ListActive(ctx)
	if err != nil {
		return
	}

	now := c.now()
	var hibernated int
	warmCount := len(activities)

	for _, activity := range activities {
		// Keep minimum warm pool
		if warmCount <= config.MinWarmPool {
			break
		}

		// During scheduled hibernation, hibernate all sandboxes that are idle
		// (even briefly idle) unless they're in the warm pool
		idleDuration := now.Sub(activity.LastActivity)
		if activity.IsIdle && idleDuration > 10*time.Second {
			if _, err := c.hypnos.Sleep(ctx, activity.ID, nil); err != nil {
				continue
			}
			hibernated++
			warmCount--
		}
	}

	if hibernated > 0 && c.metrics != nil {
		c.metrics.IncCounter("persephone_hibernations_total", float64(hibernated),
			hermes.Label{Key: "reason", Value: "scheduled"})
	}
}

// WakeForDemand wakes hibernating sandboxes to meet demand
func (c *HibernationController) WakeForDemand(ctx context.Context, count int) (int, error) {
	if c.hypnos == nil || count <= 0 {
		return 0, nil
	}

	sleeping := c.hypnos.List()
	woken := 0

	for _, record := range sleeping {
		if woken >= count {
			break
		}

		// Extract ID from record - in real implementation this would be typed
		// For now we use a type assertion pattern
		type sleepRecord interface {
			GetSandboxID() domain.SandboxID
		}

		if sr, ok := record.(sleepRecord); ok {
			if _, err := c.hypnos.Wake(ctx, sr.GetSandboxID()); err != nil {
				if c.metrics != nil {
					c.metrics.IncCounter("persephone_hibernation_errors_total", 1,
						hermes.Label{Key: "operation", Value: "wake"})
				}
				continue
			}
			woken++
		}
	}

	if woken > 0 && c.metrics != nil {
		c.metrics.IncCounter("persephone_wakes_total", float64(woken),
			hermes.Label{Key: "reason", Value: "demand"})
	}

	return woken, nil
}

// IsInHibernationWindow checks if the given time falls within a scheduled hibernation window
func (c *HibernationController) IsInHibernationWindow(config *HibernationConfig, t time.Time) bool {
	if config == nil || !config.ScheduledHibernation {
		return false
	}

	if config.HibernationStart == "" || config.HibernationEnd == "" {
		return false
	}

	if c.scheduler == nil {
		return false
	}

	// Create a temporary season to reuse the cron matching logic
	tempSeason := &Season{
		Schedule: SeasonSchedule{
			StartCron: config.HibernationStart,
			EndCron:   config.HibernationEnd,
		},
	}

	active, err := c.scheduler.ShouldActivate(tempSeason, t)
	if err != nil {
		return false
	}

	return active
}

func (c *HibernationController) checkWakeTriggers(ctx context.Context, config *HibernationConfig, now time.Time) {
	for _, trigger := range config.WakeTriggers {
		switch trigger {
		case WakeTriggerDemandSpike:
			c.checkDemandSpikeTrigger(ctx, config)
		case WakeTriggerScheduled:
			c.checkScheduledWakeTrigger(ctx, config, now)
		case WakeTriggerQueueDepth:
			// Would check queue depth metrics
		case WakeTriggerManual:
			// Manual triggers are handled via explicit API calls
		}
	}
}

func (c *HibernationController) checkDemandSpikeTrigger(ctx context.Context, config *HibernationConfig) {
	if c.scaler == nil {
		return
	}

	// Get current season's target utilization
	season, err := c.scaler.CurrentSeason(ctx)
	if err != nil || season == nil {
		return
	}

	// Check if we're over target utilization
	rec, err := c.scaler.RecommendCapacity(ctx, season.TargetUtilization)
	if err != nil {
		return
	}

	// If recommended > current, we need more capacity - wake some sandboxes
	if rec.RecommendedNodes > rec.CurrentNodes {
		deficit := rec.RecommendedNodes - rec.CurrentNodes
		c.WakeForDemand(ctx, deficit)
	}
}

func (c *HibernationController) checkScheduledWakeTrigger(ctx context.Context, config *HibernationConfig, now time.Time) {
	// Check if we're approaching the end of hibernation window
	if config.WakeLeadTime <= 0 || !c.IsInHibernationWindow(config, now) {
		return
	}

	// Check if end time is within lead time
	futureTime := now.Add(config.WakeLeadTime)
	if !c.IsInHibernationWindow(config, futureTime) {
		// We'll be exiting hibernation window soon - start pre-warming
		sleeping := c.hypnos.List()
		toWake := len(sleeping)
		if toWake > 0 {
			c.WakeForDemand(ctx, toWake)
		}
	}
}

// HibernationStatus returns the current hibernation status
type HibernationStatus struct {
	Enabled             bool
	InHibernationWindow bool
	ActiveSandboxes     int
	SleepingSandboxes   int
	MinWarmPool         int
}

// GetStatus returns the current hibernation status
func (c *HibernationController) GetStatus(ctx context.Context) *HibernationStatus {
	config := c.GetConfig()

	status := &HibernationStatus{
		Enabled: config != nil && config.Enabled,
	}

	if config != nil {
		status.InHibernationWindow = c.IsInHibernationWindow(config, c.now())
		status.MinWarmPool = config.MinWarmPool
	}

	if c.lister != nil {
		if activities, err := c.lister.ListActive(ctx); err == nil {
			status.ActiveSandboxes = len(activities)
		}
	}

	if c.hypnos != nil {
		status.SleepingSandboxes = len(c.hypnos.List())
	}

	return status
}
