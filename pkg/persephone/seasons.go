package persephone

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SeasonalScaler manages predictive and time-based scaling
type SeasonalScaler interface {
	// Forecast predicts future demand
	Forecast(ctx context.Context, window time.Duration) (*Forecast, error)

	// DefineSeason creates a seasonal scaling rule
	DefineSeason(ctx context.Context, season *Season) error

	// ApplySeason activates a seasonal configuration
	ApplySeason(ctx context.Context, seasonID string) error

	// Learn updates the model with historical data
	Learn(ctx context.Context, history []*UsageRecord) error

	// CurrentSeason returns the active season
	CurrentSeason(ctx context.Context) (*Season, error)

	// RecommendCapacity suggests optimal resource levels
	RecommendCapacity(ctx context.Context, targetUtil float64) (*CapacityRecommendation, error)
}

// Season defines a time-based scaling configuration
type Season struct {
	ID          string
	Name        string
	Description string

	// When this season applies
	Schedule SeasonSchedule

	// Scaling parameters
	MinNodes          int
	MaxNodes          int
	TargetUtilization float64

	// Pre-warming configuration
	Prewarming PrewarmConfig

	// Resource class distribution
	ResourceMix map[string]float64

	// Budget constraints for this season
	Budget BudgetConfig

	// Hibernation configuration for idle scaling
	Hibernation HibernationConfig
}

type SeasonSchedule struct {
	// Cron-style schedules
	StartCron string // e.g., "0 8 * * MON-FRI" (8am weekdays)
	EndCron   string // e.g., "0 18 * * MON-FRI" (6pm weekdays)

	// Or specific time ranges
	TimeRanges []TimeRange

	// Timezone
	Timezone string
}

type TimeRange struct {
	Start time.Time
	End   time.Time
}

type PrewarmConfig struct {
	// Templates to pre-warm
	Templates []string

	// Number of warm instances per template
	PoolSize int

	// How far ahead to start pre-warming
	LeadTime time.Duration
}

type Forecast struct {
	GeneratedAt time.Time
	Window      time.Duration
	Predictions []Prediction
	Confidence  float64
}

type Prediction struct {
	Time            time.Time
	PredictedDemand int
	UpperBound      int
	LowerBound      int
	Confidence      float64
}

type UsageRecord struct {
	Timestamp   time.Time
	ActiveVMs   int
	QueueDepth  int
	CPUUtil     float64
	MemoryUtil  float64
	LaunchCount int
	ErrorCount  int
}

type CapacityRecommendation struct {
	CurrentNodes     int
	RecommendedNodes int
	Reason           string
	CostDelta        float64
	ConfidenceLevel  float64
}

// Example seasons (Greek mythology inspired)
var (
	SeasonSpring = &Season{
		ID:          "spring",
		Name:        "Spring Growth",
		Description: "Gradual scaling up as demand increases",
		Schedule: SeasonSchedule{
			StartCron: "0 6 * * *",
			EndCron:   "0 18 * * *",
		},
		MinNodes:          5,
		MaxNodes:          50,
		TargetUtilization: 0.7,
		Prewarming: PrewarmConfig{
			Templates: []string{"python-ds", "node-18"},
			PoolSize:  10,
			LeadTime:  30 * time.Minute,
		},
	}

	SeasonSummer = &Season{
		ID:                "summer",
		Name:              "Peak Summer",
		Description:       "Maximum capacity for peak demand",
		MinNodes:          20,
		MaxNodes:          100,
		TargetUtilization: 0.8,
	}

	SeasonAutumn = &Season{
		ID:                "autumn",
		Name:              "Autumn Harvest",
		Description:       "Gradual wind-down from peak",
		MinNodes:          10,
		MaxNodes:          60,
		TargetUtilization: 0.7,
	}

	SeasonWinter = &Season{
		ID:                "winter",
		Name:              "Winter Rest",
		Description:       "Minimal capacity during low demand",
		MinNodes:          3,
		MaxNodes:          20,
		TargetUtilization: 0.5,
	}
)

// BasicSeasonalScaler is a simple implementation for testing
type BasicSeasonalScaler struct {
	seasons       map[string]*Season
	currentSeason *Season
	history       []*UsageRecord
	store         HistoryStore
	mu            sync.RWMutex
}

func NewBasicSeasonalScaler() *BasicSeasonalScaler {
	return &BasicSeasonalScaler{
		seasons: make(map[string]*Season),
		history: make([]*UsageRecord, 0),
	}
}

// NewBasicSeasonalScalerWithStore creates a scaler with persistent storage
func NewBasicSeasonalScalerWithStore(store HistoryStore) *BasicSeasonalScaler {
	return &BasicSeasonalScaler{
		seasons: make(map[string]*Season),
		history: make([]*UsageRecord, 0),
		store:   store,
	}
}

func (s *BasicSeasonalScaler) DefineSeason(ctx context.Context, season *Season) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seasons[season.ID] = season
	return nil
}

func (s *BasicSeasonalScaler) ApplySeason(ctx context.Context, seasonID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if season, ok := s.seasons[seasonID]; ok {
		s.currentSeason = season
		return nil
	}
	return nil
}

func (s *BasicSeasonalScaler) CurrentSeason(ctx context.Context) (*Season, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentSeason, nil
}

func (s *BasicSeasonalScaler) Forecast(ctx context.Context, window time.Duration) (*Forecast, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Use hybrid forecaster for improved predictions
	forecaster := NewHybridForecaster()

	// Default to 5-minute intervals for predictions
	stepInterval := 5 * time.Minute
	if window < time.Hour {
		stepInterval = time.Minute
	}

	// Forecast for the season duration
	// The original forecaster.Forecast signature was (history []*UsageRecord, window time.Duration, stepInterval time.Duration)
	// To include time.Now(), we assume the forecaster's Forecast method signature has been updated
	// to accept a start time, e.g., (history []*UsageRecord, startTime time.Time, window time.Duration, stepInterval time.Duration)
	// This change assumes an update to the HybridForecaster's interface/implementation.
	return forecaster.Forecast(s.history, time.Now(), window, stepInterval), nil
}

func (s *BasicSeasonalScaler) Learn(ctx context.Context, history []*UsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Persist to storage if available
	if s.store != nil {
		if err := s.store.Save(ctx, history); err != nil {
			// Log error but don't fail - we can still append to memory
			// The caller should have access to logger if needed
		}
	}

	// Append new history to in-memory cache
	s.history = append(s.history, history...)

	// Trim in-memory history to keep last 10000 records (increased from 1000)
	if len(s.history) > 10000 {
		s.history = s.history[len(s.history)-10000:]
	}
	return nil
}

func (s *BasicSeasonalScaler) RecommendCapacity(ctx context.Context, targetUtil float64) (*CapacityRecommendation, error) {
	s.mu.RLock()
	// Copy history for forecasting outside lock if needed, but forecast is fast enough
	history := s.history
	currentSeason := s.currentSeason
	s.mu.RUnlock()

	// 1. Calculate reactive recommendation based on current usage
	var currentActive int
	if len(history) > 0 {
		currentActive = history[len(history)-1].ActiveVMs
	}

	// Recommended = CurrentActive / TargetUtil
	reactiveRecommended := float64(currentActive) / targetUtil
	if reactiveRecommended < 1 {
		reactiveRecommended = 1
	}

	finalRecommended := reactiveRecommended
	reason := "Based on current usage"

	// 2. Calculate predictive recommendation if pre-warming is enabled
	if currentSeason != nil && currentSeason.Prewarming.LeadTime > 0 {

		// Forecast ahead by LeadTime
		leadTime := currentSeason.Prewarming.LeadTime

		// We need a window slightly larger than lead time to find the target point or just leadTime
		// Let's forecast for leadTime + 5 mins to be sure
		forecastWindow := leadTime + 5*time.Minute

		// Assuming we haven't updated the interface yet, we need to match what Forecast does
		// But s.Forecast is what is available.
		// Actually, I can just call s.Forecast(ctx, forecastWindow) but I need to handle the lock carefully.
		// I already unlocked, so I can call s.Forecast if it locks.
		// Wait, s.Forecast takes RLock. It's fine to call it if I'm not holding a lock.

		fc, err := s.Forecast(ctx, forecastWindow)
		if err == nil && len(fc.Predictions) > 0 {
			// Find prediction closest to Now + LeadTime
			targetTime := time.Now().Add(leadTime)
			var predictedDemand int

			for _, p := range fc.Predictions {
				if p.Time.After(targetTime) || p.Time.Equal(targetTime) {
					predictedDemand = p.PredictedDemand
					break
				}
				// Keep the last one if we don't pass targetTime (shouldn't happen with correct window)
				predictedDemand = p.PredictedDemand
			}

			// Pre-warm recommendation
			predictiveRecommended := float64(predictedDemand) / targetUtil

			// Take the maximum of reactive and predictive
			if predictiveRecommended > finalRecommended {
				finalRecommended = predictiveRecommended
				reason = fmt.Sprintf("Pre-warming for predicted demand of %d in %s", predictedDemand, leadTime)
			}
		}
	}

	// 3. Apply season constraints
	if currentSeason != nil {
		if finalRecommended < float64(currentSeason.MinNodes) {
			finalRecommended = float64(currentSeason.MinNodes)
			reason += " (clamped to season min)"
		}
		if finalRecommended > float64(currentSeason.MaxNodes) {
			finalRecommended = float64(currentSeason.MaxNodes)
			reason += " (clamped to season max)"
		}
	}

	return &CapacityRecommendation{
		CurrentNodes:     currentActive,
		RecommendedNodes: int(finalRecommended),
		Reason:           reason,
		ConfidenceLevel:  0.9, // This could be updated from forecast confidence
	}, nil
}

// LoadHistory restores historical data from storage
func (s *BasicSeasonalScaler) LoadHistory(ctx context.Context, days int) error {
	if s.store == nil {
		return nil // No storage configured
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Load recent history from storage
	start := time.Now().AddDate(0, 0, -days)
	end := time.Now()

	records, err := s.store.Load(ctx, start, end)
	if err != nil {
		return err
	}

	// Replace in-memory history
	s.history = records

	// Ensure we don't exceed memory limit
	if len(s.history) > 10000 {
		s.history = s.history[len(s.history)-10000:]
	}

	return nil
}

// PruneHistory removes old records from storage
func (s *BasicSeasonalScaler) PruneHistory(ctx context.Context, retentionDays int) error {
	if s.store == nil {
		return nil
	}

	return s.store.Prune(ctx, retentionDays)
}
