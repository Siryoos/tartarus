package erebus

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/require"
)

// StageTimings captures timing for each stage of the OCI conversion process
type StageTimings struct {
	ImageRef              string
	TotalDuration         time.Duration
	PullDuration          time.Duration
	LayerCount            int
	CacheHits             int
	CacheMisses           int
	ExtractionDuration    time.Duration
	InitInjectionDuration time.Duration
	ScanDuration          time.Duration
}

// instrumentedOCIBuilder wraps OCIBuilder to capture timing information
type instrumentedOCIBuilder struct {
	*OCIBuilder
	timings *StageTimings
}

// Pull wraps the original Pull to capture timing
func (ib *instrumentedOCIBuilder) Pull(ctx context.Context, ref string) (v1.Image, error) {
	start := time.Now()
	img, err := ib.OCIBuilder.Pull(ctx, ref)
	ib.timings.PullDuration = time.Since(start)
	return img, err
}

// Assemble wraps the original Assemble to capture detailed stage timings
func (ib *instrumentedOCIBuilder) Assemble(ctx context.Context, ref string, outputDir string) error {
	overallStart := time.Now()

	// Pull image
	pullStart := time.Now()
	img, err := ib.Pull(ctx, ref)
	if err != nil {
		return err
	}
	ib.timings.PullDuration = time.Since(pullStart)

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	ib.timings.LayerCount = len(layers)

	// Track cache hits/misses and extraction time
	extractStart := time.Now()

	// Check cache status for each layer
	for _, layer := range layers {
		digest, err := layer.Digest()
		if err != nil {
			continue
		}
		key := fmt.Sprintf("layers/%s", digest.Hex)
		exists, err := ib.Store.Exists(ctx, key)
		if err != nil {
			continue
		}
		if exists {
			ib.timings.CacheHits++
		} else {
			ib.timings.CacheMisses++
		}
	}

	// Call the original Assemble which does all the real work
	// We use the parent's Assemble to avoid recursion
	if err := ib.OCIBuilder.Assemble(ctx, ref, outputDir); err != nil {
		return err
	}

	ib.timings.ExtractionDuration = time.Since(extractStart)
	ib.timings.TotalDuration = time.Since(overallStart)

	return nil
}

// TestPerformanceAnalysis performs comprehensive profiling of OCI conversion
func TestPerformanceAnalysis(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance analysis in short mode")
	}

	testImages := []string{
		"python:3.11-slim",
		"node:18",
		"ubuntu:22.04",
	}

	results := make([]StageTimings, 0, len(testImages)*2)

	for _, imageRef := range testImages {
		t.Run(imageRef, func(t *testing.T) {
			// Test 1: Cold cache
			t.Run("ColdCache", func(t *testing.T) {
				cacheDir, err := os.MkdirTemp("", "erebus-perf-cold")
				require.NoError(t, err)
				defer os.RemoveAll(cacheDir)

				store, err := NewLocalStore(cacheDir)
				require.NoError(t, err)

				timings := &StageTimings{ImageRef: imageRef + " (cold)"}
				builder := &instrumentedOCIBuilder{
					OCIBuilder: NewOCIBuilder(store, nil),
					timings:    timings,
				}
				builder.Scanner = nil // Disable scanner for cleaner timing

				outDir, err := os.MkdirTemp("", "erebus-perf-out")
				require.NoError(t, err)
				defer os.RemoveAll(outDir)

				err = builder.Assemble(context.Background(), imageRef, outDir)
				require.NoError(t, err)

				results = append(results, *timings)

				// Log results
				t.Logf("Cold Cache Results for %s:", imageRef)
				t.Logf("  Total Duration: %v", timings.TotalDuration)
				t.Logf("  Pull Duration: %v (%.1f%%)", timings.PullDuration,
					100.0*timings.PullDuration.Seconds()/timings.TotalDuration.Seconds())
				t.Logf("  Extraction Duration: %v (%.1f%%)", timings.ExtractionDuration,
					100.0*timings.ExtractionDuration.Seconds()/timings.TotalDuration.Seconds())
				t.Logf("  Layer Count: %d", timings.LayerCount)
				t.Logf("  Cache Hits: %d", timings.CacheHits)
				t.Logf("  Cache Misses: %d", timings.CacheMisses)
			})

			// Test 2: Warm cache
			t.Run("WarmCache", func(t *testing.T) {
				cacheDir, err := os.MkdirTemp("", "erebus-perf-warm")
				require.NoError(t, err)
				defer os.RemoveAll(cacheDir)

				store, err := NewLocalStore(cacheDir)
				require.NoError(t, err)

				baseBuilder := NewOCIBuilder(store, nil)
				baseBuilder.Scanner = nil

				// Warmup run
				warmupDir, err := os.MkdirTemp("", "erebus-warmup")
				require.NoError(t, err)
				err = baseBuilder.Assemble(context.Background(), imageRef, warmupDir)
				require.NoError(t, err)
				os.RemoveAll(warmupDir)

				// Instrumented run
				timings := &StageTimings{ImageRef: imageRef + " (warm)"}
				builder := &instrumentedOCIBuilder{
					OCIBuilder: baseBuilder,
					timings:    timings,
				}

				outDir, err := os.MkdirTemp("", "erebus-perf-out")
				require.NoError(t, err)
				defer os.RemoveAll(outDir)

				err = builder.Assemble(context.Background(), imageRef, outDir)
				require.NoError(t, err)

				results = append(results, *timings)

				// Log results
				t.Logf("Warm Cache Results for %s:", imageRef)
				t.Logf("  Total Duration: %v", timings.TotalDuration)
				t.Logf("  Pull Duration: %v (%.1f%%)", timings.PullDuration,
					100.0*timings.PullDuration.Seconds()/timings.TotalDuration.Seconds())
				t.Logf("  Extraction Duration: %v (%.1f%%)", timings.ExtractionDuration,
					100.0*timings.ExtractionDuration.Seconds()/timings.TotalDuration.Seconds())
				t.Logf("  Layer Count: %d", timings.LayerCount)
				t.Logf("  Cache Hits: %d", timings.CacheHits)
				t.Logf("  Cache Misses: %d", timings.CacheMisses)

				cacheHitRatio := 0.0
				if timings.LayerCount > 0 {
					cacheHitRatio = 100.0 * float64(timings.CacheHits) / float64(timings.LayerCount)
				}
				t.Logf("  Cache Hit Ratio: %.1f%%", cacheHitRatio)

				// Assert warm cache is using cache effectively
				require.Greater(t, timings.CacheHits, 0, "Warm cache should have cache hits")
				require.GreaterOrEqual(t, cacheHitRatio, 50.0, "Cache hit ratio should be at least 50%")
			})
		})
	}

	// Print summary table
	t.Log("\n=== Performance Summary ===")
	t.Log("Image                          | Cache  | Total    | Pull     | Extract  | Layers | Hit% ")
	t.Log("------------------------------|--------|----------|----------|----------|--------|------")
	for _, timing := range results {
		cacheHitPct := 0.0
		if timing.LayerCount > 0 {
			cacheHitPct = 100.0 * float64(timing.CacheHits) / float64(timing.LayerCount)
		}
		t.Logf("%-29s | %-6s | %8.2fs | %8.2fs | %8.2fs | %6d | %5.1f%%",
			timing.ImageRef,
			"",
			timing.TotalDuration.Seconds(),
			timing.PullDuration.Seconds(),
			timing.ExtractionDuration.Seconds(),
			timing.LayerCount,
			cacheHitPct,
		)
	}

	// Check 30s SLO for warm cache
	t.Log("\n=== SLO Check: <30s Target ===")
	for _, timing := range results {
		if timing.ImageRef[len(timing.ImageRef)-6:] == "(warm)" {
			passed := timing.TotalDuration.Seconds() < 30.0
			status := "✓ PASS"
			if !passed {
				status = "✗ FAIL"
			}
			t.Logf("%s: %8.2fs - %s", timing.ImageRef, timing.TotalDuration.Seconds(), status)
		}
	}
}
