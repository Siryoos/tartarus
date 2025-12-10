package persephone

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CronScheduler evaluates cron expressions and activates seasons automatically
type CronScheduler struct {
	location *time.Location
}

// NewCronScheduler creates a scheduler with the given timezone
func NewCronScheduler(timezone string) (*CronScheduler, error) {
	if timezone == "" {
		timezone = "UTC"
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %s: %w", timezone, err)
	}

	return &CronScheduler{
		location: loc,
	}, nil
}

// ShouldActivate checks if a season should be active at the given time
func (s *CronScheduler) ShouldActivate(season *Season, t time.Time) (bool, error) {
	// Convert to season's timezone
	t = t.In(s.location)

	// Check cron-based schedule
	if season.Schedule.StartCron != "" && season.Schedule.EndCron != "" {
		return s.matchesCronSchedule(season, t)
	}

	// Check time range schedule
	if len(season.Schedule.TimeRanges) > 0 {
		return s.matchesTimeRange(season, t), nil
	}

	return false, nil
}

func (s *CronScheduler) matchesCronSchedule(season *Season, t time.Time) (bool, error) {
	// Simple cron matching for common patterns
	// Format: "minute hour day month weekday"
	// Example: "0 8 * * MON-FRI" = 8am on weekdays

	startHour := s.extractHour(season.Schedule.StartCron)
	endHour := s.extractHour(season.Schedule.EndCron)

	// Check if weekday matches
	startParts := strings.Fields(season.Schedule.StartCron)
	if len(startParts) >= 5 {
		weekdayPattern := startParts[4]
		if weekdayPattern != "*" && !s.matchesWeekday(weekdayPattern, t.Weekday()) {
			return false, nil
		}
	}

	// Check if current hour is within the active window
	currentHour := t.Hour()
	if startHour <= endHour {
		// Normal case: 9am-5pm
		return currentHour >= startHour && currentHour < endHour, nil
	}
	// Wrap-around case: 10pm-2am
	return currentHour >= startHour || currentHour < endHour, nil
}

func (s *CronScheduler) matchCron(cronExpr string, t time.Time) (bool, error) {
	parts := strings.Fields(cronExpr)
	if len(parts) != 5 {
		return false, fmt.Errorf("invalid cron expression: %s", cronExpr)
	}

	minute := parts[0]
	hour := parts[1]
	// day := parts[2]
	// month := parts[3]
	weekday := parts[4]

	// Check minute
	if minute != "*" {
		// Parse minute value
		var targetMinute int
		fmt.Sscanf(minute, "%d", &targetMinute)
		if t.Minute() != targetMinute {
			return false, nil
		}
	}

	// Check hour
	if hour != "*" {
		var targetHour int
		fmt.Sscanf(hour, "%d", &targetHour)
		if t.Hour() != targetHour {
			return false, nil
		}
	}

	// Check weekday (simplified: MON-FRI, SAT-SUN, or specific day)
	if weekday != "*" {
		if !s.matchesWeekday(weekday, t.Weekday()) {
			return false, nil
		}
	}

	return true, nil
}

func (s *CronScheduler) matchesWeekday(pattern string, weekday time.Weekday) bool {
	pattern = strings.ToUpper(pattern)

	// Handle ranges like MON-FRI
	if strings.Contains(pattern, "-") {
		return s.inWeekdayRange(pattern, weekday)
	}

	// Handle specific days
	dayMap := map[string]time.Weekday{
		"SUN": time.Sunday,
		"MON": time.Monday,
		"TUE": time.Tuesday,
		"WED": time.Wednesday,
		"THU": time.Thursday,
		"FRI": time.Friday,
		"SAT": time.Saturday,
	}

	targetDay, ok := dayMap[pattern]
	if !ok {
		return false
	}

	return weekday == targetDay
}

func (s *CronScheduler) inWeekdayRange(pattern string, weekday time.Weekday) bool {
	parts := strings.Split(pattern, "-")
	if len(parts) != 2 {
		return false
	}

	dayMap := map[string]int{
		"SUN": 0, "MON": 1, "TUE": 2, "WED": 3,
		"THU": 4, "FRI": 5, "SAT": 6,
	}

	start, okStart := dayMap[strings.TrimSpace(parts[0])]
	end, okEnd := dayMap[strings.TrimSpace(parts[1])]

	if !okStart || !okEnd {
		return false
	}

	currentDay := int(weekday)

	// Handle wrap-around (e.g., SAT-MON)
	if start <= end {
		return currentDay >= start && currentDay <= end
	}
	return currentDay >= start || currentDay <= end
}

func (s *CronScheduler) extractHour(cronExpr string) int {
	parts := strings.Fields(cronExpr)
	if len(parts) < 2 {
		return 0
	}
	var hour int
	fmt.Sscanf(parts[1], "%d", &hour)
	return hour
}

func (s *CronScheduler) matchesTimeRange(season *Season, t time.Time) bool {
	for _, tr := range season.Schedule.TimeRanges {
		if (t.Equal(tr.Start) || t.After(tr.Start)) && (t.Equal(tr.End) || t.Before(tr.End)) {
			return true
		}
	}
	return false
}

// SeasonActivator manages automatic season transitions
type SeasonActivator struct {
	scheduler             *CronScheduler
	seasons               map[string]*Season
	current               *Season
	hibernationController *HibernationController
}

// NewSeasonActivator creates an activator with the given scheduler
func NewSeasonActivator(scheduler *CronScheduler) *SeasonActivator {
	return &SeasonActivator{
		scheduler: scheduler,
		seasons:   make(map[string]*Season),
	}
}

// SetHibernationController sets the hibernation controller for season transitions
func (a *SeasonActivator) SetHibernationController(controller *HibernationController) {
	a.hibernationController = controller
}

// RegisterSeason adds a season to the activator
func (a *SeasonActivator) RegisterSeason(season *Season) {
	a.seasons[season.ID] = season
}

// EvaluateSeasons checks which season should be active now
func (a *SeasonActivator) EvaluateSeasons(ctx context.Context, t time.Time) (*Season, error) {
	// Find the season with the highest priority that matches
	// Priority: explicit time ranges > cron schedules

	var bestMatch *Season

	for _, season := range a.seasons {
		active, err := a.scheduler.ShouldActivate(season, t)
		if err != nil {
			continue
		}

		if active {
			if bestMatch == nil || a.hasPriority(season, bestMatch) {
				bestMatch = season
			}
		}
	}

	// Only transition if different from current
	if bestMatch != nil && (a.current == nil || a.current.ID != bestMatch.ID) {
		a.current = bestMatch

		// Trigger hibernation controller if transitioning to a hibernation-enabled season
		if bestMatch.Hibernation.Enabled && a.hibernationController != nil {
			go a.hibernationController.EvaluateHibernation(ctx, bestMatch)
		}

		return bestMatch, nil
	}

	return a.current, nil
}

func (a *SeasonActivator) hasPriority(s1, s2 *Season) bool {
	// Explicit time ranges have priority
	if len(s1.Schedule.TimeRanges) > 0 && len(s2.Schedule.TimeRanges) == 0 {
		return true
	}
	return false
}

// GetCurrentSeason returns the currently active season
func (a *SeasonActivator) GetCurrentSeason() *Season {
	return a.current
}
