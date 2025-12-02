package perf

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/typhon"
)

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
