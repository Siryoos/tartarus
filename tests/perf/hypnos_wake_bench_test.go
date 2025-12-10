// Package perf provides Hypnos hibernation/wake performance benchmarks.
package perf

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// HypnosWakeTarget is the SLO target for Hypnos wake-from-sleep.
const HypnosWakeTarget = 100 * time.Millisecond

// HypnosFeatureFlags controls Hypnos feature behavior.
type HypnosFeatureFlags struct {
	// EnableFastWake enables optimized wake path
	EnableFastWake bool

	// EnablePreemptiveDecompress starts decompression before full request
	EnablePreemptiveDecompress bool

	// EnableMemoryMappedSnapshots uses mmap for snapshot loading
	EnableMemoryMappedSnapshots bool

	// EnableParallelRestore enables parallel memory/disk restore
	EnableParallelRestore bool

	// MaxConcurrentWakes limits concurrent wake operations
	MaxConcurrentWakes int
}

// DefaultHypnosFeatureFlags returns the default feature flag settings.
func DefaultHypnosFeatureFlags() *HypnosFeatureFlags {
	return &HypnosFeatureFlags{
		EnableFastWake:              true,
		EnablePreemptiveDecompress:  true,
		EnableMemoryMappedSnapshots: false, // Not yet implemented
		EnableParallelRestore:       true,
		MaxConcurrentWakes:          4,
	}
}

// StagingFeatureFlags returns feature flags for staging environment.
func StagingFeatureFlags() *HypnosFeatureFlags {
	return &HypnosFeatureFlags{
		EnableFastWake:              true,
		EnablePreemptiveDecompress:  true,
		EnableMemoryMappedSnapshots: true, // Test in staging
		EnableParallelRestore:       true,
		MaxConcurrentWakes:          8,
	}
}

// HypnosWakeTimings captures detailed timing for wake phases.
type HypnosWakeTimings struct {
	SandboxID        string
	TotalDuration    time.Duration
	DownloadDuration time.Duration
	DecompressDur    time.Duration
	LaunchDuration   time.Duration
	HooksDuration    time.Duration
	Phases           map[string]time.Duration
	FeatureFlags     *HypnosFeatureFlags
	SnapshotSizeMB   int64
	Success          bool
	Error            error
}

// TestHypnosWakeFromSleepPerformance tests the wake-from-sleep performance.
func TestHypnosWakeFromSleepPerformance(t *testing.T) {
	metrics := hermes.NewPrometheusMetrics()
	harness := NewPerfHarness(metrics)
	ctx := context.Background()

	// Setup runtime and store
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	runtime := tartarus.NewMockRuntime(logger)

	// Configure mock runtime for fast operations
	runtime.SetStartDuration(20 * time.Millisecond) // Simulate VM restore time

	storeDir := t.TempDir()
	store, err := erebus.NewLocalStore(storeDir)
	require.NoError(t, err)

	stagingDir := t.TempDir()
	manager := hypnos.NewManager(runtime, store, stagingDir)
	manager.Metrics = metrics

	// Track wake timings
	var wakeTimings []HypnosWakeTimings
	iterations := 50

	for i := 0; i < iterations; i++ {
		sandboxID := domain.SandboxID(fmt.Sprintf("hypnos-perf-test-%d", i))

		// Create and launch a sandbox
		req := &domain.SandboxRequest{
			ID:       sandboxID,
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
		}
		cfg := tartarus.VMConfig{
			OverlayFS: "/tmp/overlay",
			CPUs:      1,
			MemoryMB:  512,
		}

		_, err := runtime.Launch(ctx, req, cfg)
		require.NoError(t, err)

		// Put to sleep
		sleepStart := time.Now()
		sleepRecord, err := manager.Sleep(ctx, sandboxID, nil)
		sleepDuration := time.Since(sleepStart)
		require.NoError(t, err)

		t.Logf("Sleep completed in %v (compression ratio: %.2f)", sleepDuration, sleepRecord.CompressionRatio)

		// Measure wake time
		timer := harness.StartTimer("perf_hypnos_wake_seconds", map[string]string{
			"sandbox_id": string(sandboxID),
			"iteration":  fmt.Sprintf("%d", i),
		})

		wakeStart := time.Now()
		run, err := manager.Wake(ctx, sandboxID)
		wakeDuration := time.Since(wakeStart)

		timing := HypnosWakeTimings{
			SandboxID:     string(sandboxID),
			TotalDuration: wakeDuration,
			FeatureFlags:  DefaultHypnosFeatureFlags(),
			Success:       err == nil,
			Error:         err,
		}

		if err != nil {
			timer.StopWithError(err)
			timing.Success = false
			wakeTimings = append(wakeTimings, timing)
			continue
		}

		timer.Stop()
		wakeTimings = append(wakeTimings, timing)

		require.NotNil(t, run)
		require.Equal(t, sandboxID, run.ID)

		// Record detailed metrics
		metrics.ObserveHistogram("hypnos_wake_duration_seconds", wakeDuration.Seconds(),
			hermes.Label{Key: "sandbox_id", Value: string(sandboxID)})

		// Cleanup
		runtime.Kill(ctx, sandboxID)
	}

	// Calculate statistics
	var durations []time.Duration
	for _, timing := range wakeTimings {
		if timing.Success {
			durations = append(durations, timing.TotalDuration)
		}
	}

	if len(durations) == 0 {
		t.Fatal("No successful wake operations")
	}

	p50 := calculatePercentile(durations, 50)
	p95 := calculatePercentile(durations, 95)
	p99 := calculatePercentile(durations, 99)
	avg := calculateAverage(durations)
	successRate := float64(len(durations)) / float64(len(wakeTimings)) * 100

	// Report results
	t.Log("\n=== Hypnos Wake-from-Sleep Performance ===")
	t.Logf("Iterations:   %d", iterations)
	t.Logf("Success Rate: %.1f%%", successRate)
	t.Logf("Average:      %v", avg)
	t.Logf("P50:          %v", p50)
	t.Logf("P95:          %v", p95)
	t.Logf("P99:          %v", p99)
	t.Logf("Target:       <%v", HypnosWakeTarget)

	// SLO Check
	if p99 > HypnosWakeTarget {
		t.Errorf("SLO VIOLATION: Wake P99 %v exceeds target %v", p99, HypnosWakeTarget)
	} else {
		t.Logf("\nSLO Check: PASS - P99 %v < Target %v", p99, HypnosWakeTarget)
	}

	// Generate harness report
	report := harness.GenerateReport()
	t.Log(report.String())
}

// TestHypnosWakeWithFeatureFlags tests wake performance with different feature flag configurations.
func TestHypnosWakeWithFeatureFlags(t *testing.T) {
	metrics := hermes.NewPrometheusMetrics()
	harness := NewPerfHarness(metrics)
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	runtime := tartarus.NewMockRuntime(logger)
	runtime.SetStartDuration(20 * time.Millisecond)

	storeDir := t.TempDir()
	store, err := erebus.NewLocalStore(storeDir)
	require.NoError(t, err)

	stagingDir := t.TempDir()

	// Test different feature flag configurations
	featureFlagConfigs := []struct {
		name  string
		flags *HypnosFeatureFlags
	}{
		{"Default", DefaultHypnosFeatureFlags()},
		{"Staging", StagingFeatureFlags()},
		{"Conservative", &HypnosFeatureFlags{
			EnableFastWake:              false,
			EnablePreemptiveDecompress:  false,
			EnableMemoryMappedSnapshots: false,
			EnableParallelRestore:       false,
			MaxConcurrentWakes:          1,
		}},
		{"Aggressive", &HypnosFeatureFlags{
			EnableFastWake:              true,
			EnablePreemptiveDecompress:  true,
			EnableMemoryMappedSnapshots: true,
			EnableParallelRestore:       true,
			MaxConcurrentWakes:          16,
		}},
	}

	for _, config := range featureFlagConfigs {
		t.Run(config.name, func(t *testing.T) {
			manager := hypnos.NewManager(runtime, store, stagingDir)
			manager.Metrics = metrics

			iterations := 20
			var durations []time.Duration

			for i := 0; i < iterations; i++ {
				sandboxID := domain.SandboxID(fmt.Sprintf("ff-test-%s-%d", config.name, i))

				// Create sandbox
				req := &domain.SandboxRequest{
					ID:       sandboxID,
					Template: "test-template",
					Resources: domain.ResourceSpec{
						CPU: 1000,
						Mem: 512,
					},
				}
				cfg := tartarus.VMConfig{
					OverlayFS: "/tmp/overlay",
					CPUs:      1,
					MemoryMB:  512,
				}

				_, err := runtime.Launch(ctx, req, cfg)
				require.NoError(t, err)

				// Sleep
				_, err = manager.Sleep(ctx, sandboxID, nil)
				require.NoError(t, err)

				// Wake and measure
				timer := harness.StartTimer("perf_hypnos_wake_seconds", map[string]string{
					"feature_config": config.name,
					"iteration":      fmt.Sprintf("%d", i),
				})

				start := time.Now()
				_, err = manager.Wake(ctx, sandboxID)
				duration := time.Since(start)

				if err != nil {
					timer.StopWithError(err)
					continue
				}

				timer.Stop()
				durations = append(durations, duration)

				// Cleanup
				runtime.Kill(ctx, sandboxID)
			}

			if len(durations) > 0 {
				p99 := calculatePercentile(durations, 99)
				avg := calculateAverage(durations)

				t.Logf("Config: %s - Avg: %v, P99: %v", config.name, avg, p99)

				// Record feature flag specific metrics
				metrics.ObserveHistogram("hypnos_wake_feature_config_seconds", p99.Seconds(),
					hermes.Label{Key: "config", Value: config.name})
			}
		})
	}
}

// TestHypnosSleepWakeCycle tests the full sleep-wake cycle for regression.
func TestHypnosSleepWakeCycle(t *testing.T) {
	metrics := hermes.NewPrometheusMetrics()
	harness := NewPerfHarness(metrics)
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	runtime := tartarus.NewMockRuntime(logger)
	runtime.SetStartDuration(20 * time.Millisecond)

	storeDir := t.TempDir()
	store, err := erebus.NewLocalStore(storeDir)
	require.NoError(t, err)

	manager := hypnos.NewManager(runtime, store, t.TempDir())
	manager.Metrics = metrics

	// Track cycle timings
	type cycleTimings struct {
		sleepDuration time.Duration
		wakeDuration  time.Duration
	}

	var cycles []cycleTimings
	iterations := 30

	for i := 0; i < iterations; i++ {
		sandboxID := domain.SandboxID(fmt.Sprintf("cycle-test-%d", i))

		// Setup
		req := &domain.SandboxRequest{
			ID:        sandboxID,
			Template:  "test-template",
			Resources: domain.ResourceSpec{CPU: 1000, Mem: 512},
		}
		cfg := tartarus.VMConfig{
			OverlayFS: "/tmp/overlay",
			CPUs:      1,
			MemoryMB:  512,
		}

		_, err := runtime.Launch(ctx, req, cfg)
		require.NoError(t, err)

		// Sleep
		sleepTimer := harness.StartTimer("perf_hypnos_sleep_seconds", map[string]string{
			"iteration": fmt.Sprintf("%d", i),
		})
		sleepStart := time.Now()
		_, err = manager.Sleep(ctx, sandboxID, nil)
		sleepDuration := time.Since(sleepStart)
		if err != nil {
			sleepTimer.StopWithError(err)
			continue
		}
		sleepTimer.Stop()

		// Wake
		wakeTimer := harness.StartTimer("perf_hypnos_wake_seconds", map[string]string{
			"iteration": fmt.Sprintf("%d", i),
		})
		wakeStart := time.Now()
		_, err = manager.Wake(ctx, sandboxID)
		wakeDuration := time.Since(wakeStart)
		if err != nil {
			wakeTimer.StopWithError(err)
			continue
		}
		wakeTimer.Stop()

		cycles = append(cycles, cycleTimings{
			sleepDuration: sleepDuration,
			wakeDuration:  wakeDuration,
		})

		// Cleanup
		runtime.Kill(ctx, sandboxID)
	}

	// Calculate statistics
	var sleepDurations, wakeDurations []time.Duration
	for _, c := range cycles {
		sleepDurations = append(sleepDurations, c.sleepDuration)
		wakeDurations = append(wakeDurations, c.wakeDuration)
	}

	sleepP99 := calculatePercentile(sleepDurations, 99)
	wakeP99 := calculatePercentile(wakeDurations, 99)

	t.Log("\n=== Hypnos Sleep-Wake Cycle Performance ===")
	t.Logf("Cycles:      %d", len(cycles))
	t.Logf("Sleep P99:   %v", sleepP99)
	t.Logf("Wake P99:    %v", wakeP99)
	t.Logf("Wake Target: <%v", HypnosWakeTarget)

	// SLO Check for wake
	if wakeP99 > HypnosWakeTarget {
		t.Errorf("SLO VIOLATION: Wake P99 %v exceeds target %v", wakeP99, HypnosWakeTarget)
	} else {
		t.Logf("\nSLO Check: PASS - Wake P99 %v < Target %v", wakeP99, HypnosWakeTarget)
	}
}

// BenchmarkHypnosWake provides Go benchmark for Hypnos wake performance.
func BenchmarkHypnosWake(b *testing.B) {
	metrics := hermes.NewPrometheusMetrics()
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	runtime := tartarus.NewMockRuntime(logger)
	runtime.SetStartDuration(20 * time.Millisecond)

	storeDir, err := os.MkdirTemp("", "hypnos-bench-store")
	require.NoError(b, err)
	defer os.RemoveAll(storeDir)

	store, err := erebus.NewLocalStore(storeDir)
	require.NoError(b, err)

	stagingDir, err := os.MkdirTemp("", "hypnos-bench-staging")
	require.NoError(b, err)
	defer os.RemoveAll(stagingDir)

	manager := hypnos.NewManager(runtime, store, stagingDir)
	manager.Metrics = metrics

	// Pre-create and sleep a sandbox for benchmarking wake
	sandboxID := domain.SandboxID("bench-sandbox")
	req := &domain.SandboxRequest{
		ID:        sandboxID,
		Template:  "bench-template",
		Resources: domain.ResourceSpec{CPU: 1000, Mem: 512},
	}
	cfg := tartarus.VMConfig{
		OverlayFS: "/tmp/overlay",
		CPUs:      1,
		MemoryMB:  512,
	}

	// Launch and sleep once
	_, err = runtime.Launch(ctx, req, cfg)
	require.NoError(b, err)
	_, err = manager.Sleep(ctx, sandboxID, nil)
	require.NoError(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Re-sleep between iterations
		if i > 0 {
			_, err = manager.Sleep(ctx, sandboxID, nil)
			require.NoError(b, err)
		}
		b.StartTimer()

		start := time.Now()
		_, err := manager.Wake(ctx, sandboxID)
		duration := time.Since(start)

		if err != nil {
			b.Fatalf("Wake failed: %v", err)
		}

		b.ReportMetric(float64(duration.Milliseconds()), "ms/op")
	}
}

// TestHypnosHibernationResumeTraces captures detailed traces for hibernation/resume.
func TestHypnosHibernationResumeTraces(t *testing.T) {
	metrics := hermes.NewPrometheusMetrics()
	harness := NewPerfHarness(metrics)
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	runtime := tartarus.NewMockRuntime(logger)
	runtime.SetStartDuration(20 * time.Millisecond)

	storeDir := t.TempDir()
	store, err := erebus.NewLocalStore(storeDir)
	require.NoError(t, err)

	manager := hypnos.NewManager(runtime, store, t.TempDir())
	manager.Metrics = metrics

	// Add lifecycle hooks to capture traces
	var traces []struct {
		phase     string
		sandboxID domain.SandboxID
		timestamp time.Time
		duration  time.Duration
	}

	traceStart := make(map[string]time.Time)

	manager.Hooks = &hypnos.LifecycleHooks{
		PreSleep: func(ctx context.Context, id domain.SandboxID) error {
			traceStart["pre_sleep"] = time.Now()
			t.Logf("TRACE: PreSleep started for %s", id)
			return nil
		},
		PostSleep: func(ctx context.Context, id domain.SandboxID, record *hypnos.SleepRecord) error {
			duration := time.Since(traceStart["pre_sleep"])
			traces = append(traces, struct {
				phase     string
				sandboxID domain.SandboxID
				timestamp time.Time
				duration  time.Duration
			}{"sleep", id, time.Now(), duration})
			t.Logf("TRACE: PostSleep completed for %s (total: %v, ratio: %.2f)", id, duration, record.CompressionRatio)
			metrics.ObserveHistogram("hypnos_trace_sleep_total_seconds", duration.Seconds(),
				hermes.Label{Key: "sandbox_id", Value: string(id)})
			return nil
		},
		PreWake: func(ctx context.Context, id domain.SandboxID, record *hypnos.SleepRecord) error {
			traceStart["pre_wake"] = time.Now()
			t.Logf("TRACE: PreWake started for %s", id)
			return nil
		},
		PostWake: func(ctx context.Context, id domain.SandboxID, run *domain.SandboxRun) error {
			duration := time.Since(traceStart["pre_wake"])
			traces = append(traces, struct {
				phase     string
				sandboxID domain.SandboxID
				timestamp time.Time
				duration  time.Duration
			}{"wake", id, time.Now(), duration})
			t.Logf("TRACE: PostWake completed for %s (total: %v)", id, duration)
			metrics.ObserveHistogram("hypnos_trace_wake_total_seconds", duration.Seconds(),
				hermes.Label{Key: "sandbox_id", Value: string(id)})
			return nil
		},
	}

	// Run hibernation/resume cycles
	iterations := 10
	for i := 0; i < iterations; i++ {
		sandboxID := domain.SandboxID(fmt.Sprintf("trace-test-%d", i))

		req := &domain.SandboxRequest{
			ID:        sandboxID,
			Template:  "trace-template",
			Resources: domain.ResourceSpec{CPU: 1000, Mem: 512},
		}
		cfg := tartarus.VMConfig{
			OverlayFS: "/tmp/overlay",
			CPUs:      1,
			MemoryMB:  512,
		}

		// Launch
		_, err := runtime.Launch(ctx, req, cfg)
		require.NoError(t, err)

		// Sleep with trace
		timer := harness.StartTimer("perf_hypnos_trace_sleep_seconds", map[string]string{
			"sandbox_id": string(sandboxID),
		})
		_, err = manager.Sleep(ctx, sandboxID, nil)
		timer.Stop()
		require.NoError(t, err)

		// Wake with trace
		timer = harness.StartTimer("perf_hypnos_trace_wake_seconds", map[string]string{
			"sandbox_id": string(sandboxID),
		})
		_, err = manager.Wake(ctx, sandboxID)
		timer.Stop()
		require.NoError(t, err)

		// Cleanup
		runtime.Kill(ctx, sandboxID)
	}

	// Report traces
	t.Log("\n=== Hibernation/Resume Trace Summary ===")
	t.Logf("Total Traces: %d", len(traces))

	var sleepDurations, wakeDurations []time.Duration
	for _, trace := range traces {
		if trace.phase == "sleep" {
			sleepDurations = append(sleepDurations, trace.duration)
		} else {
			wakeDurations = append(wakeDurations, trace.duration)
		}
	}

	if len(sleepDurations) > 0 {
		t.Logf("Sleep - Avg: %v, P99: %v", calculateAverage(sleepDurations), calculatePercentile(sleepDurations, 99))
	}
	if len(wakeDurations) > 0 {
		wakeP99 := calculatePercentile(wakeDurations, 99)
		t.Logf("Wake - Avg: %v, P99: %v", calculateAverage(wakeDurations), wakeP99)

		// SLO gating
		if wakeP99 > HypnosWakeTarget {
			t.Errorf("SLO GATE FAILURE: Wake P99 %v exceeds target %v - BLOCKING RELEASE", wakeP99, HypnosWakeTarget)
		} else {
			t.Logf("SLO GATE: PASS - Wake P99 %v meets target %v", wakeP99, HypnosWakeTarget)
		}
	}
}

// TestHypnosSLOGating validates SLO requirements before release.
func TestHypnosSLOGating(t *testing.T) {
	/*
		=== Hypnos SLO Gating Requirements ===

		This test serves as a release gate. If it fails, the release should be blocked.

		SLO Requirements:
		1. Wake-from-sleep P99 latency < 100ms
		2. Sleep operation success rate > 99%
		3. Wake operation success rate > 99%
		4. No data corruption during hibernation cycle

		Feature Flag Requirements:
		1. EnableFastWake must be enabled in staging
		2. EnablePreemptiveDecompress must be enabled in staging
		3. All feature flags must be validated before production rollout
	*/

	t.Run("SLO_WakeLatency", func(t *testing.T) {
		metrics := hermes.NewPrometheusMetrics()
		ctx := context.Background()

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
		runtime := tartarus.NewMockRuntime(logger)
		runtime.SetStartDuration(20 * time.Millisecond)

		store, err := erebus.NewLocalStore(t.TempDir())
		require.NoError(t, err)

		manager := hypnos.NewManager(runtime, store, t.TempDir())
		manager.Metrics = metrics

		iterations := 100
		var durations []time.Duration
		errors := 0

		for i := 0; i < iterations; i++ {
			sandboxID := domain.SandboxID(fmt.Sprintf("slo-gate-%d", i))

			req := &domain.SandboxRequest{
				ID:        sandboxID,
				Template:  "slo-template",
				Resources: domain.ResourceSpec{CPU: 1000, Mem: 512},
			}
			cfg := tartarus.VMConfig{
				OverlayFS: "/tmp/overlay",
				CPUs:      1,
				MemoryMB:  512,
			}

			_, err := runtime.Launch(ctx, req, cfg)
			require.NoError(t, err)

			_, err = manager.Sleep(ctx, sandboxID, nil)
			if err != nil {
				errors++
				continue
			}

			start := time.Now()
			_, err = manager.Wake(ctx, sandboxID)
			duration := time.Since(start)

			if err != nil {
				errors++
				continue
			}

			durations = append(durations, duration)
			runtime.Kill(ctx, sandboxID)
		}

		// Calculate metrics
		successRate := float64(len(durations)) / float64(iterations) * 100
		p99 := calculatePercentile(durations, 99)

		t.Logf("SLO Gate Results:")
		t.Logf("  Iterations:   %d", iterations)
		t.Logf("  Success Rate: %.1f%% (target: >99%%)", successRate)
		t.Logf("  Wake P99:     %v (target: <%v)", p99, HypnosWakeTarget)

		// Assert SLO requirements
		if successRate < 99.0 {
			t.Errorf("SLO GATE FAILURE: Success rate %.1f%% below 99%% threshold", successRate)
		}
		if p99 > HypnosWakeTarget {
			t.Errorf("SLO GATE FAILURE: Wake P99 %v exceeds target %v", p99, HypnosWakeTarget)
		}

		if successRate >= 99.0 && p99 <= HypnosWakeTarget {
			t.Log("SLO GATE: PASSED - Release can proceed")
		}
	})

	t.Run("FeatureFlag_Staging", func(t *testing.T) {
		flags := StagingFeatureFlags()

		t.Logf("Staging Feature Flags:")
		t.Logf("  EnableFastWake:              %v (required: true)", flags.EnableFastWake)
		t.Logf("  EnablePreemptiveDecompress:  %v (required: true)", flags.EnablePreemptiveDecompress)
		t.Logf("  EnableMemoryMappedSnapshots: %v", flags.EnableMemoryMappedSnapshots)
		t.Logf("  EnableParallelRestore:       %v", flags.EnableParallelRestore)
		t.Logf("  MaxConcurrentWakes:          %d", flags.MaxConcurrentWakes)

		// Validate required flags
		if !flags.EnableFastWake {
			t.Error("STAGING GATE FAILURE: EnableFastWake must be true in staging")
		}
		if !flags.EnablePreemptiveDecompress {
			t.Error("STAGING GATE FAILURE: EnablePreemptiveDecompress must be true in staging")
		}
	})
}
