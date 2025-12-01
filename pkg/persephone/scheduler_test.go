package persephone

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCronScheduler_Weekdays(t *testing.T) {
	scheduler, err := NewCronScheduler("UTC")
	require.NoError(t, err)

	season := &Season{
		ID:   "business-hours",
		Name: "Business Hours",
		Schedule: SeasonSchedule{
			StartCron: "0 8 * * MON-FRI",
			EndCron:   "0 18 * * MON-FRI",
		},
	}

	// Monday 8:00 AM - should activate
	monday8am := time.Date(2025, 1, 6, 8, 0, 0, 0, time.UTC) // Monday
	active, err := scheduler.ShouldActivate(season, monday8am)
	require.NoError(t, err)
	assert.True(t, active)

	// Monday 2:00 PM - should be active
	monday2pm := time.Date(2025, 1, 6, 14, 0, 0, 0, time.UTC)
	active, err = scheduler.ShouldActivate(season, monday2pm)
	require.NoError(t, err)
	assert.True(t, active)

	// Saturday 10:00 AM - should NOT be active
	saturday10am := time.Date(2025, 1, 4, 10, 0, 0, 0, time.UTC) // Saturday
	active, err = scheduler.ShouldActivate(season, saturday10am)
	require.NoError(t, err)
	assert.False(t, active)
}

func TestCronScheduler_TimeRange(t *testing.T) {
	scheduler, err := NewCronScheduler("UTC")
	require.NoError(t, err)

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

	season := &Season{
		ID:   "january-promo",
		Name: "January Promotion",
		Schedule: SeasonSchedule{
			TimeRanges: []TimeRange{
				{Start: start, End: end},
			},
		},
	}

	// January 15th - should be active
	jan15 := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	active, err := scheduler.ShouldActivate(season, jan15)
	require.NoError(t, err)
	assert.True(t, active)

	// February 1st - should NOT be active
	feb1 := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	active, err = scheduler.ShouldActivate(season, feb1)
	require.NoError(t, err)
	assert.False(t, active)
}

func TestSeasonActivator(t *testing.T) {
	scheduler, err := NewCronScheduler("UTC")
	require.NoError(t, err)

	activator := NewSeasonActivator(scheduler)

	businessHours := &Season{
		ID:   "business",
		Name: "Business Hours",
		Schedule: SeasonSchedule{
			StartCron: "0 9 * * MON-FRI",
			EndCron:   "0 17 * * MON-FRI",
		},
	}

	weekend := &Season{
		ID:   "weekend",
		Name: "Weekend",
		Schedule: SeasonSchedule{
			StartCron: "0 0 * * SAT-SUN",
			EndCron:   "0 23 * * SAT-SUN",
		},
	}

	activator.RegisterSeason(businessHours)
	activator.RegisterSeason(weekend)

	// Monday 10am - should activate business hours
	monday10am := time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC)
	season, err := activator.EvaluateSeasons(context.Background(), monday10am)
	require.NoError(t, err)
	assert.NotNil(t, season)
	assert.Equal(t, "business", season.ID)

	// Saturday 10am - should activate weekend
	saturday10am := time.Date(2025, 1, 4, 10, 0, 0, 0, time.UTC)
	season, err = activator.EvaluateSeasons(context.Background(), saturday10am)
	require.NoError(t, err)
	assert.NotNil(t, season)
	assert.Equal(t, "weekend", season.ID)
}

func TestCronScheduler_Timezone(t *testing.T) {
	// Test with PST timezone
	scheduler, err := NewCronScheduler("America/Los_Angeles")
	require.NoError(t, err)

	season := &Season{
		ID: "morning",
		Schedule: SeasonSchedule{
			StartCron: "0 9 * * *",  // 9 AM PST
			EndCron:   "0 17 * * *", // 5 PM PST
		},
	}

	// 9 AM PST on January 15, 2025
	pstLoc, _ := time.LoadLocation("America/Los_Angeles")
	pst9am := time.Date(2025, 1, 15, 9, 0, 0, 0, pstLoc)

	active, err := scheduler.ShouldActivate(season, pst9am)
	require.NoError(t, err)
	assert.True(t, active, "Should be active at 9 AM PST")

	// 10 AM PST - should still be active
	pst10am := time.Date(2025, 1, 15, 10, 0, 0, 0, pstLoc)
	active, err = scheduler.ShouldActivate(season, pst10am)
	require.NoError(t, err)
	assert.True(t, active, "Should be active at 10 AM PST")
}

func TestWeekdayRange(t *testing.T) {
	scheduler, err := NewCronScheduler("UTC")
	require.NoError(t, err)

	// Test MON-FRI range
	assert.True(t, scheduler.matchesWeekday("MON-FRI", time.Monday))
	assert.True(t, scheduler.matchesWeekday("MON-FRI", time.Wednesday))
	assert.True(t, scheduler.matchesWeekday("MON-FRI", time.Friday))
	assert.False(t, scheduler.matchesWeekday("MON-FRI", time.Saturday))
	assert.False(t, scheduler.matchesWeekday("MON-FRI", time.Sunday))

	// Test SAT-SUN range
	assert.True(t, scheduler.matchesWeekday("SAT-SUN", time.Saturday))
	assert.True(t, scheduler.matchesWeekday("SAT-SUN", time.Sunday))
	assert.False(t, scheduler.matchesWeekday("SAT-SUN", time.Monday))
}
