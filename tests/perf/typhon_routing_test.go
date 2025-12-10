package perf

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/typhon"
)

// TyphonQuarantineOverheadTarget is the SLO target for quarantine routing overhead.
const TyphonQuarantineOverheadTarget = 50 * time.Millisecond

// BenchmarkTyphonRouting measures the overhead of the decision path for quarantine routing.
// It compares the "Normal" path vs "Quarantined" path.
func BenchmarkTyphonRouting(b *testing.B) {
	// Setup
	ctx := context.Background()
	qm := typhon.NewInMemoryQuarantineManager()

	// Create a quarantined sandbox record
	quarantinedID := "sandbox-quarantined-1"
	_, err := qm.Quarantine(ctx, &typhon.QuarantineRequest{
		SandboxID:   quarantinedID,
		Reason:      typhon.ReasonSuspiciousBehavior,
		RequestedBy: "benchmark",
	})
	require.NoError(b, err)

	normalID := "sandbox-normal-1"

	b.Run("Normal_Routing", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// 1. Check if quarantined (Routing decision)
			records, err := qm.ListQuarantined(ctx, &typhon.QuarantineFilter{SandboxID: normalID, Status: typhon.StatusActive})
			if err != nil {
				b.Fatal(err)
			}
			isQuarantined := len(records) > 0

			// 2. Select Profile
			var profile *typhon.SeccompProfile
			if isQuarantined {
				profile, _ = typhon.GetQuarantineProfile()
			} else {
				profile, _ = typhon.GetDefaultProfile()
			}

			if profile == nil {
				b.Fatal("profile is nil")
			}
		}
	})

	b.Run("Quarantine_Routing", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// 1. Check if quarantined (Routing decision)
			records, err := qm.ListQuarantined(ctx, &typhon.QuarantineFilter{SandboxID: quarantinedID, Status: typhon.StatusActive})
			if err != nil {
				b.Fatal(err)
			}
			isQuarantined := len(records) > 0

			// 2. Select Profile
			var profile *typhon.SeccompProfile
			if isQuarantined {
				profile, _ = typhon.GetQuarantineProfile()
			} else {
				profile, _ = typhon.GetDefaultProfile()
			}

			if profile == nil {
				b.Fatal("profile is nil")
			}
		}
	})

	// Measure just the profile loading part to see if that's the bottleneck
	b.Run("GetProfile_Default", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = typhon.GetDefaultProfile()
		}
	})

	b.Run("GetProfile_Quarantine", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = typhon.GetQuarantineProfile()
		}
	})
}

// TestTyphonQuarantineOverheadMeasurement measures and reports quarantine routing overhead.
func TestTyphonQuarantineOverheadMeasurement(t *testing.T) {
	metrics := hermes.NewPrometheusMetrics()
	harness := NewPerfHarness(metrics)
	ctx := context.Background()

	qm := typhon.NewInMemoryQuarantineManager()

	// Setup: Create some quarantined sandboxes
	for i := 0; i < 10; i++ {
		_, err := qm.Quarantine(ctx, &typhon.QuarantineRequest{
			SandboxID:   fmt.Sprintf("quarantine-test-%d", i),
			Reason:      typhon.ReasonSuspiciousBehavior,
			RequestedBy: "test",
		})
		require.NoError(t, err)
	}

	iterations := 1000
	var normalDurations []time.Duration
	var quarantineDurations []time.Duration

	// Measure normal routing path
	t.Run("NormalRouting", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			sandboxID := fmt.Sprintf("normal-sandbox-%d", i)

			timer := harness.StartTimer("perf_typhon_normal_routing_seconds", map[string]string{
				"sandbox": sandboxID,
			})

			start := time.Now()

			// Routing decision: check quarantine status
			records, err := qm.ListQuarantined(ctx, &typhon.QuarantineFilter{
				SandboxID: sandboxID,
				Status:    typhon.StatusActive,
			})
			require.NoError(t, err)
			isQuarantined := len(records) > 0

			// Profile selection
			var profile *typhon.SeccompProfile
			if isQuarantined {
				profile, _ = typhon.GetQuarantineProfile()
			} else {
				profile, _ = typhon.GetDefaultProfile()
			}
			require.NotNil(t, profile)

			duration := time.Since(start)
			timer.Stop()
			normalDurations = append(normalDurations, duration)
		}
	})

	// Measure quarantine routing path
	t.Run("QuarantineRouting", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			sandboxID := fmt.Sprintf("quarantine-test-%d", i%10)

			timer := harness.StartTimer("perf_typhon_quarantine_routing_seconds", map[string]string{
				"sandbox": sandboxID,
			})

			start := time.Now()

			// Routing decision: check quarantine status
			records, err := qm.ListQuarantined(ctx, &typhon.QuarantineFilter{
				SandboxID: sandboxID,
				Status:    typhon.StatusActive,
			})
			require.NoError(t, err)
			isQuarantined := len(records) > 0

			// Profile selection (should be quarantine profile)
			var profile *typhon.SeccompProfile
			if isQuarantined {
				profile, _ = typhon.GetQuarantineProfile()
			} else {
				profile, _ = typhon.GetDefaultProfile()
			}
			require.NotNil(t, profile)
			require.True(t, isQuarantined, "Expected quarantine status for quarantine-test sandbox")

			duration := time.Since(start)
			timer.Stop()
			quarantineDurations = append(quarantineDurations, duration)
		}
	})

	// Calculate overhead
	normalP99 := calculatePercentile(normalDurations, 99)
	quarantineP99 := calculatePercentile(quarantineDurations, 99)
	overhead := quarantineP99 - normalP99

	// Record overhead metric
	harness.RecordResult("perf_typhon_quarantine_overhead_seconds", overhead, map[string]string{
		"type": "p99_overhead",
	})

	// Report results
	t.Log("\n=== Typhon Quarantine Routing Overhead Analysis ===")
	t.Logf("Normal Routing P99:      %v", normalP99)
	t.Logf("Quarantine Routing P99:  %v", quarantineP99)
	t.Logf("Overhead (P99 diff):     %v", overhead)
	t.Logf("Target:                  <%v", TyphonQuarantineOverheadTarget)

	// Calculate additional statistics
	normalAvg := calculateAverage(normalDurations)
	quarantineAvg := calculateAverage(quarantineDurations)

	t.Logf("\nDetailed Statistics:")
	t.Logf("  Normal Path:")
	t.Logf("    Average: %v", normalAvg)
	t.Logf("    P50:     %v", calculatePercentile(normalDurations, 50))
	t.Logf("    P95:     %v", calculatePercentile(normalDurations, 95))
	t.Logf("    P99:     %v", normalP99)
	t.Logf("  Quarantine Path:")
	t.Logf("    Average: %v", quarantineAvg)
	t.Logf("    P50:     %v", calculatePercentile(quarantineDurations, 50))
	t.Logf("    P95:     %v", calculatePercentile(quarantineDurations, 95))
	t.Logf("    P99:     %v", quarantineP99)

	// SLO Check
	if overhead > TyphonQuarantineOverheadTarget {
		t.Errorf("SLO VIOLATION: Quarantine routing overhead %v exceeds target %v", overhead, TyphonQuarantineOverheadTarget)
	} else {
		t.Logf("\nSLO Check: PASS - Overhead %v < Target %v", overhead, TyphonQuarantineOverheadTarget)
	}

	// Generate report
	report := harness.GenerateReport()
	t.Log(report.String())
}

// TestTyphonSeccompIsolationLatency tests the latency impact of seccomp profile application.
func TestTyphonSeccompIsolationLatency(t *testing.T) {
	metrics := hermes.NewPrometheusMetrics()
	harness := NewPerfHarness(metrics)

	iterations := 100

	// Measure default profile loading time
	t.Run("DefaultProfileLoad", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			timer := harness.StartTimer("perf_typhon_default_profile_load_seconds", nil)
			profile, err := typhon.GetDefaultProfile()
			timer.Stop()
			require.NoError(t, err)
			require.NotNil(t, profile)
		}
	})

	// Measure quarantine profile loading time
	t.Run("QuarantineProfileLoad", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			timer := harness.StartTimer("perf_typhon_quarantine_profile_load_seconds", nil)
			profile, err := typhon.GetQuarantineProfile()
			timer.Stop()
			require.NoError(t, err)
			require.NotNil(t, profile)
		}
	})

	// Generate report
	report := harness.GenerateReport()
	t.Log("\n=== Seccomp Profile Loading Performance ===")
	t.Log(report.String())
}

// TestTyphonQuarantineDecisionPath tests the full quarantine decision path latency.
func TestTyphonQuarantineDecisionPath(t *testing.T) {
	metrics := hermes.NewPrometheusMetrics()
	harness := NewPerfHarness(metrics)
	ctx := context.Background()

	qm := typhon.NewInMemoryQuarantineManager()

	// Create a set of policies and triggers
	policy := &typhon.QuarantinePolicy{
		AutoTriggers: []typhon.AutoQuarantineTrigger{
			{
				Condition: "risk_score > 0.8",
				Reason:    typhon.ReasonSuspiciousBehavior,
				Enabled:   true,
			},
			{
				Condition: "syscall_violations > 10",
				Reason:    typhon.ReasonPolicyViolation,
				Enabled:   true,
			},
		},
		DefaultNetworkMode:    typhon.NetworkModeNone,
		MaxQuarantineDuration: 24 * time.Hour,
		Isolation: typhon.QuarantineIsolation{
			NetworkMode:    typhon.NetworkModeNone,
			SeccompProfile: "quarantine-strict",
			EnableStrace:   true,
			EnableAuditd:   true,
			RecordNetwork:  true,
		},
	}
	require.NoError(t, qm.SetPolicy(ctx, policy))

	// Populate with various quarantine records
	reasons := []typhon.QuarantineReason{
		typhon.ReasonSuspiciousBehavior,
		typhon.ReasonPolicyViolation,
		typhon.ReasonNetworkAnomaly,
		typhon.ReasonResourceAbuse,
		typhon.ReasonUntrustedSource,
	}

	for i := 0; i < 100; i++ {
		reason := reasons[i%len(reasons)]
		_, err := qm.Quarantine(ctx, &typhon.QuarantineRequest{
			SandboxID:      fmt.Sprintf("sandbox-%d", i),
			Reason:         reason,
			RequestedBy:    "test-harness",
			AutoQuarantine: i%2 == 0,
			Hardened:       i%3 == 0,
		})
		require.NoError(t, err)
	}

	iterations := 500
	var decisionDurations []time.Duration

	t.Run("FullDecisionPath", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			sandboxID := fmt.Sprintf("sandbox-%d", i%100)

			timer := harness.StartTimer("perf_typhon_decision_path_seconds", map[string]string{
				"sandbox_id": sandboxID,
			})

			start := time.Now()

			// Step 1: Check quarantine status
			records, err := qm.ListQuarantined(ctx, &typhon.QuarantineFilter{
				SandboxID: sandboxID,
				Status:    typhon.StatusActive,
			})
			require.NoError(t, err)
			isQuarantined := len(records) > 0

			// Step 2: Get appropriate profile
			var profile *typhon.SeccompProfile
			if isQuarantined {
				profile, _ = typhon.GetQuarantineProfile()
			} else {
				profile, _ = typhon.GetDefaultProfile()
			}

			// Step 3: Determine network configuration
			var networkMode typhon.NetworkMode
			if isQuarantined && len(records) > 0 {
				networkMode = typhon.NetworkModeNone
			} else {
				networkMode = typhon.NetworkModeMonitored
			}

			// Step 4: Prepare isolation config (simulated)
			_ = profile
			_ = networkMode

			duration := time.Since(start)
			timer.Stop()
			decisionDurations = append(decisionDurations, duration)
		}
	})

	// Report statistics
	p50 := calculatePercentile(decisionDurations, 50)
	p95 := calculatePercentile(decisionDurations, 95)
	p99 := calculatePercentile(decisionDurations, 99)
	avg := calculateAverage(decisionDurations)

	t.Log("\n=== Full Quarantine Decision Path Performance ===")
	t.Logf("Iterations: %d", iterations)
	t.Logf("Average:    %v", avg)
	t.Logf("P50:        %v", p50)
	t.Logf("P95:        %v", p95)
	t.Logf("P99:        %v", p99)

	// SLO Check: decision path should be <50ms
	if p99 > TyphonQuarantineOverheadTarget {
		t.Errorf("SLO VIOLATION: Decision path P99 %v exceeds target %v", p99, TyphonQuarantineOverheadTarget)
	} else {
		t.Logf("SLO Check: PASS - P99 %v < Target %v", p99, TyphonQuarantineOverheadTarget)
	}
}

// calculateAverage calculates the average duration.
func calculateAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	return sum / time.Duration(len(durations))
}
