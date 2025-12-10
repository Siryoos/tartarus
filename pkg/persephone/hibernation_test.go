package persephone

import (
	"context"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// mockHypnosManager implements HypnosManager for testing
type mockHypnosManager struct {
	sleeping map[domain.SandboxID]bool
	sleepErr error
	wakeErr  error
}

func newMockHypnosManager() *mockHypnosManager {
	return &mockHypnosManager{
		sleeping: make(map[domain.SandboxID]bool),
	}
}

func (m *mockHypnosManager) Sleep(ctx context.Context, id domain.SandboxID, opts interface{}) (interface{}, error) {
	if m.sleepErr != nil {
		return nil, m.sleepErr
	}
	m.sleeping[id] = true
	return nil, nil
}

func (m *mockHypnosManager) Wake(ctx context.Context, id domain.SandboxID) (interface{}, error) {
	if m.wakeErr != nil {
		return nil, m.wakeErr
	}
	delete(m.sleeping, id)
	return nil, nil
}

func (m *mockHypnosManager) List() []interface{} {
	result := make([]interface{}, 0, len(m.sleeping))
	for id := range m.sleeping {
		result = append(result, &mockSleepRecord{id: id})
	}
	return result
}

func (m *mockHypnosManager) IsSleeping(id domain.SandboxID) bool {
	return m.sleeping[id]
}

type mockSleepRecord struct {
	id domain.SandboxID
}

func (r *mockSleepRecord) GetSandboxID() domain.SandboxID {
	return r.id
}

// mockSandboxLister implements SandboxLister for testing
type mockSandboxLister struct {
	activities []SandboxActivity
}

func (m *mockSandboxLister) ListActive(ctx context.Context) ([]SandboxActivity, error) {
	return m.activities, nil
}

func TestHibernationController_HibernateIdle(t *testing.T) {
	hypnos := newMockHypnosManager()
	now := time.Now()

	lister := &mockSandboxLister{
		activities: []SandboxActivity{
			{ID: "sandbox-1", LastActivity: now.Add(-30 * time.Minute), IsIdle: true},
			{ID: "sandbox-2", LastActivity: now.Add(-5 * time.Minute), IsIdle: true},
			{ID: "sandbox-3", LastActivity: now, IsIdle: false}, // Not idle
		},
	}

	controller := NewHibernationController(hypnos, lister, nil, nil, nil)
	controller.now = func() time.Time { return now }

	ctx := context.Background()

	// Hibernate sandboxes idle for more than 10 minutes
	hibernated, err := controller.HibernateIdle(ctx, 10*time.Minute)
	if err != nil {
		t.Fatalf("HibernateIdle failed: %v", err)
	}

	// Should hibernate sandbox-1 (30 min idle) but not sandbox-2 (5 min) or sandbox-3 (not idle)
	if hibernated != 1 {
		t.Errorf("Expected 1 hibernated, got %d", hibernated)
	}

	if !hypnos.IsSleeping("sandbox-1") {
		t.Error("sandbox-1 should be sleeping")
	}
	if hypnos.IsSleeping("sandbox-2") {
		t.Error("sandbox-2 should not be sleeping")
	}
	if hypnos.IsSleeping("sandbox-3") {
		t.Error("sandbox-3 should not be sleeping")
	}
}

func TestHibernationController_WakeForDemand(t *testing.T) {
	hypnos := newMockHypnosManager()

	// Pre-hibernate some sandboxes
	hypnos.sleeping["sandbox-1"] = true
	hypnos.sleeping["sandbox-2"] = true
	hypnos.sleeping["sandbox-3"] = true

	controller := NewHibernationController(hypnos, nil, nil, nil, nil)
	ctx := context.Background()

	// Wake 2 sandboxes
	woken, err := controller.WakeForDemand(ctx, 2)
	if err != nil {
		t.Fatalf("WakeForDemand failed: %v", err)
	}

	if woken != 2 {
		t.Errorf("Expected 2 woken, got %d", woken)
	}

	// Should have 1 still sleeping
	if len(hypnos.sleeping) != 1 {
		t.Errorf("Expected 1 still sleeping, got %d", len(hypnos.sleeping))
	}
}

func TestHibernationController_IsInHibernationWindow(t *testing.T) {
	scheduler, _ := NewCronScheduler("UTC")
	controller := NewHibernationController(nil, nil, nil, scheduler, nil)

	config := &HibernationConfig{
		Enabled:              true,
		ScheduledHibernation: true,
		HibernationStart:     "0 22 * * *", // 10pm
		HibernationEnd:       "0 6 * * *",  // 6am
	}

	// Test at 11pm - should be in hibernation
	t11pm := time.Date(2024, 1, 15, 23, 0, 0, 0, time.UTC)
	if !controller.IsInHibernationWindow(config, t11pm) {
		t.Error("11pm should be in hibernation window")
	}

	// Test at 3am - should be in hibernation
	t3am := time.Date(2024, 1, 15, 3, 0, 0, 0, time.UTC)
	if !controller.IsInHibernationWindow(config, t3am) {
		t.Error("3am should be in hibernation window")
	}

	// Test at 10am - should not be in hibernation
	t10am := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	if controller.IsInHibernationWindow(config, t10am) {
		t.Error("10am should not be in hibernation window")
	}
}

func TestHibernationController_MinWarmPool(t *testing.T) {
	hypnos := newMockHypnosManager()
	now := time.Now()

	// All sandboxes are idle
	lister := &mockSandboxLister{
		activities: []SandboxActivity{
			{ID: "sandbox-1", LastActivity: now.Add(-30 * time.Minute), IsIdle: true},
			{ID: "sandbox-2", LastActivity: now.Add(-30 * time.Minute), IsIdle: true},
			{ID: "sandbox-3", LastActivity: now.Add(-30 * time.Minute), IsIdle: true},
		},
	}

	scheduler, _ := NewCronScheduler("UTC")
	controller := NewHibernationController(hypnos, lister, nil, scheduler, nil)
	controller.now = func() time.Time { return now }

	// Set config with min warm pool of 2
	config := &HibernationConfig{
		Enabled:              true,
		ScheduledHibernation: true,
		HibernationStart:     "0 0 * * *",
		HibernationEnd:       "0 23 * * *",
		MinWarmPool:          2,
	}
	controller.SetConfig(config)

	ctx := context.Background()

	// Trigger scheduled hibernation
	controller.hibernateForSchedule(ctx, config)

	// Should only hibernate 1 (keeping 2 warm)
	sleepingCount := len(hypnos.sleeping)
	if sleepingCount != 1 {
		t.Errorf("Expected 1 hibernated (keeping 2 warm), got %d", sleepingCount)
	}
}

func TestHibernationController_GetStatus(t *testing.T) {
	hypnos := newMockHypnosManager()
	hypnos.sleeping["sandbox-1"] = true

	lister := &mockSandboxLister{
		activities: []SandboxActivity{
			{ID: "sandbox-2", LastActivity: time.Now(), IsIdle: false},
			{ID: "sandbox-3", LastActivity: time.Now(), IsIdle: false},
		},
	}

	controller := NewHibernationController(hypnos, lister, nil, nil, nil)

	config := &HibernationConfig{
		Enabled:     true,
		MinWarmPool: 2,
	}
	controller.SetConfig(config)

	ctx := context.Background()
	status := controller.GetStatus(ctx)

	if !status.Enabled {
		t.Error("Expected enabled to be true")
	}
	if status.ActiveSandboxes != 2 {
		t.Errorf("Expected 2 active sandboxes, got %d", status.ActiveSandboxes)
	}
	if status.SleepingSandboxes != 1 {
		t.Errorf("Expected 1 sleeping sandbox, got %d", status.SleepingSandboxes)
	}
	if status.MinWarmPool != 2 {
		t.Errorf("Expected min warm pool 2, got %d", status.MinWarmPool)
	}
}

func TestHibernationConfig_Disabled(t *testing.T) {
	controller := NewHibernationController(nil, nil, nil, nil, nil)

	// Nil config
	if controller.IsInHibernationWindow(nil, time.Now()) {
		t.Error("Nil config should not be in hibernation window")
	}

	// Disabled config
	config := &HibernationConfig{Enabled: false}
	controller.SetConfig(config)

	ctx := context.Background()
	controller.evaluate(ctx) // Should not panic
}
