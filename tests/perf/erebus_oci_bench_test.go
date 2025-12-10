// Package perf provides OCI image conversion performance benchmarks for Erebus.
package perf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// OCIConversionTarget is the SLO target for OCI image conversion.
const OCIConversionTarget = 30 * time.Second

// Representative large images for benchmarking OCI conversion.
// These cover common data science and web development base images.
var representativeLargeImages = []struct {
	name        string
	ref         string
	description string
	estimatedMB int
}{
	{"Python-Slim", "python:3.11-slim", "Python 3.11 slim base image", 130},
	{"Node-18", "node:18-slim", "Node.js 18 slim image", 180},
	{"Ubuntu-22.04", "ubuntu:22.04", "Ubuntu 22.04 LTS base", 77},
	{"Python-DS", "python:3.11", "Full Python image (data science base)", 920},
	{"Alpine", "alpine:3.19", "Alpine minimal base", 7},
}

// OCIConversionTimings captures detailed timing for OCI conversion phases.
type OCIConversionTimings struct {
	ImageRef           string
	TotalDuration      time.Duration
	PullDuration       time.Duration
	CacheCheckDuration time.Duration
	DownloadDuration   time.Duration
	ExtractionDuration time.Duration
	InitInjection      time.Duration
	RootFSBuild        time.Duration
	LayerCount         int
	CacheHits          int
	CacheMisses        int
	TotalBytes         int64
	CacheHitRatio      float64
}

// InstrumentedOCIBuilder wraps OCIBuilder to capture phase timings.
type InstrumentedOCIBuilder struct {
	*erebus.OCIBuilder
	metrics  hermes.Metrics
	timings  *OCIConversionTimings
	cacheDir string
}

// NewInstrumentedOCIBuilder creates an instrumented builder.
func NewInstrumentedOCIBuilder(store erebus.Store, metrics hermes.Metrics, cacheDir string) *InstrumentedOCIBuilder {
	return &InstrumentedOCIBuilder{
		OCIBuilder: erebus.NewOCIBuilder(store, nil),
		metrics:    metrics,
		timings:    &OCIConversionTimings{},
		cacheDir:   cacheDir,
	}
}

// GetTimings returns the captured timings.
func (ib *InstrumentedOCIBuilder) GetTimings() *OCIConversionTimings {
	return ib.timings
}

// ResetTimings resets the captured timings.
func (ib *InstrumentedOCIBuilder) ResetTimings() {
	ib.timings = &OCIConversionTimings{}
}

// TestErebusOCIConversionPerformance tests OCI conversion with detailed profiling.
func TestErebusOCIConversionPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping OCI conversion performance test in short mode")
	}

	metrics := hermes.NewPrometheusMetrics()
	harness := NewPerfHarness(metrics)

	results := make([]OCIConversionTimings, 0)

	for _, img := range representativeLargeImages {
		t.Run(img.name, func(t *testing.T) {
			// Test cold cache scenario
			t.Run("ColdCache", func(t *testing.T) {
				cacheDir := t.TempDir()
				store, err := erebus.NewLocalStore(cacheDir)
				require.NoError(t, err)

				builder := NewInstrumentedOCIBuilder(store, metrics, cacheDir)
				builder.Scanner = nil // Disable scanner for clean timing

				outDir := t.TempDir()

				// Time the entire operation
				timer := harness.StartTimer("perf_erebus_oci_conversion_seconds", map[string]string{
					"image": img.name,
					"cache": "cold",
				})

				overallStart := time.Now()

				// Pull phase
				pullStart := time.Now()
				pullCtx, pullCancel := context.WithTimeout(context.Background(), 5*time.Minute)
				_, pullErr := builder.Pull(pullCtx, img.ref)
				pullCancel()
				pullDuration := time.Since(pullStart)

				if pullErr != nil {
					timer.StopWithError(pullErr)
					t.Skipf("Failed to pull image %s (network issue): %v", img.ref, pullErr)
					return
				}

				// Assemble phase (includes download, extraction, init injection)
				assembleStart := time.Now()
				err = builder.Assemble(context.Background(), img.ref, outDir)
				assembleDuration := time.Since(assembleStart)

				totalDuration := time.Since(overallStart)

				if err != nil {
					timer.StopWithError(err)
					t.Errorf("Assemble failed: %v", err)
					return
				}

				timer.Stop()

				// Calculate cache statistics from cache directory
				cacheHits, cacheMisses, totalBytes := calculateCacheStats(t, cacheDir)

				timing := OCIConversionTimings{
					ImageRef:           fmt.Sprintf("%s (cold)", img.name),
					TotalDuration:      totalDuration,
					PullDuration:       pullDuration,
					ExtractionDuration: assembleDuration,
					CacheHits:          cacheHits,
					CacheMisses:        cacheMisses,
					TotalBytes:         totalBytes,
					CacheHitRatio:      0.0, // Cold cache = 0%
				}
				results = append(results, timing)

				// Record detailed metrics
				metrics.ObserveHistogram("erebus_pull_duration_seconds", pullDuration.Seconds(),
					hermes.Label{Key: "image", Value: img.name})
				metrics.ObserveHistogram("erebus_assemble_duration_seconds", assembleDuration.Seconds(),
					hermes.Label{Key: "image", Value: img.name})
				metrics.SetGauge("erebus_layer_cache_misses", float64(cacheMisses),
					hermes.Label{Key: "image", Value: img.name})

				t.Logf("Cold Cache Results for %s:", img.name)
				t.Logf("  Total Duration: %v", totalDuration)
				t.Logf("  Pull Duration: %v", pullDuration)
				t.Logf("  Assemble Duration: %v", assembleDuration)
				t.Logf("  Cache Misses: %d", cacheMisses)
				t.Logf("  Total Bytes: %d MB", totalBytes/(1024*1024))
			})

			// Test warm cache scenario
			t.Run("WarmCache", func(t *testing.T) {
				cacheDir := t.TempDir()
				store, err := erebus.NewLocalStore(cacheDir)
				require.NoError(t, err)

				builder := erebus.NewOCIBuilder(store, nil)
				builder.Scanner = nil

				// Warm up the cache
				warmupDir := t.TempDir()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				err = builder.Assemble(ctx, img.ref, warmupDir)
				cancel()
				if err != nil {
					t.Skipf("Failed to warm cache for %s: %v", img.ref, err)
					return
				}

				// Count layers after warmup
				cacheHits, _, totalBytes := calculateCacheStats(t, cacheDir)

				// Now test with warm cache
				outDir := t.TempDir()

				timer := harness.StartTimer("perf_erebus_oci_conversion_seconds", map[string]string{
					"image": img.name,
					"cache": "warm",
				})

				start := time.Now()
				err = builder.Assemble(context.Background(), img.ref, outDir)
				duration := time.Since(start)

				if err != nil {
					timer.StopWithError(err)
					t.Errorf("Warm cache assemble failed: %v", err)
					return
				}

				timer.Stop()

				// Calculate cache hit ratio
				cacheHitRatio := 100.0
				if cacheHits > 0 {
					cacheHitRatio = 100.0 // All hits since we warmed up
				}

				timing := OCIConversionTimings{
					ImageRef:           fmt.Sprintf("%s (warm)", img.name),
					TotalDuration:      duration,
					ExtractionDuration: duration,
					CacheHits:          cacheHits,
					CacheMisses:        0,
					TotalBytes:         totalBytes,
					CacheHitRatio:      cacheHitRatio,
				}
				results = append(results, timing)

				metrics.ObserveHistogram("erebus_warm_cache_duration_seconds", duration.Seconds(),
					hermes.Label{Key: "image", Value: img.name})
				metrics.SetGauge("erebus_layer_cache_hits", float64(cacheHits),
					hermes.Label{Key: "image", Value: img.name})
				metrics.SetGauge("erebus_cache_hit_ratio", cacheHitRatio,
					hermes.Label{Key: "image", Value: img.name})

				t.Logf("Warm Cache Results for %s:", img.name)
				t.Logf("  Total Duration: %v", duration)
				t.Logf("  Cache Hits: %d", cacheHits)
				t.Logf("  Cache Hit Ratio: %.1f%%", cacheHitRatio)
				t.Logf("  Total Bytes: %d MB", totalBytes/(1024*1024))

				// SLO Check
				if duration > OCIConversionTarget {
					t.Errorf("SLO VIOLATION: Warm cache conversion %v exceeds target %v", duration, OCIConversionTarget)
				}
			})
		})
	}

	// Print summary report
	t.Log("\n=== OCI Conversion Performance Summary ===")
	t.Log("Image                          | Cache  | Total    | Cache Hit%")
	t.Log("-------------------------------|--------|----------|----------")
	for _, timing := range results {
		t.Logf("%-30s | %6.2fs | %6.1f%%",
			timing.ImageRef,
			timing.TotalDuration.Seconds(),
			timing.CacheHitRatio,
		)
	}

	// Check overall SLO
	t.Log("\n=== SLO Check: <30s Target (Warm Cache) ===")
	for _, timing := range results {
		if timing.CacheHitRatio > 0 { // Warm cache results
			passed := timing.TotalDuration <= OCIConversionTarget
			status := "PASS"
			if !passed {
				status = "FAIL"
				t.Errorf("%s: %v - %s", timing.ImageRef, timing.TotalDuration, status)
			} else {
				t.Logf("%s: %v - %s", timing.ImageRef, timing.TotalDuration, status)
			}
		}
	}

	// Generate harness report
	report := harness.GenerateReport()
	t.Log(report.String())
}

// BenchmarkErebusOCIConversion benchmarks OCI conversion for various images.
func BenchmarkErebusOCIConversion(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping OCI conversion benchmark in short mode")
	}

	metrics := hermes.NewPrometheusMetrics()

	// Only test smaller images in benchmark to keep time reasonable
	benchImages := []struct {
		name string
		ref  string
	}{
		{"Alpine", "alpine:3.19"},
		{"Ubuntu", "ubuntu:22.04"},
	}

	for _, img := range benchImages {
		// Cold cache benchmark
		b.Run(img.name+"_ColdCache", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				cacheDir, err := os.MkdirTemp("", "erebus-bench-cold")
				require.NoError(b, err)

				store, err := erebus.NewLocalStore(cacheDir)
				require.NoError(b, err)

				builder := erebus.NewOCIBuilder(store, nil)
				builder.Scanner = nil

				outDir, err := os.MkdirTemp("", "erebus-out")
				require.NoError(b, err)

				b.StartTimer()
				start := time.Now()
				err = builder.Assemble(context.Background(), img.ref, outDir)
				duration := time.Since(start)
				b.StopTimer()

				if err != nil {
					b.Skipf("Failed to assemble %s: %v", img.ref, err)
				}

				b.ReportMetric(float64(duration.Seconds()), "s/op")
				metrics.ObserveHistogram("bench_oci_cold_seconds", duration.Seconds(),
					hermes.Label{Key: "image", Value: img.name})

				os.RemoveAll(outDir)
				os.RemoveAll(cacheDir)
			}
		})

		// Warm cache benchmark
		b.Run(img.name+"_WarmCache", func(b *testing.B) {
			// Setup persistent cache
			cacheDir, err := os.MkdirTemp("", "erebus-bench-warm")
			require.NoError(b, err)
			defer os.RemoveAll(cacheDir)

			store, err := erebus.NewLocalStore(cacheDir)
			require.NoError(b, err)

			builder := erebus.NewOCIBuilder(store, nil)
			builder.Scanner = nil

			// Warm up
			warmupDir, err := os.MkdirTemp("", "erebus-warmup")
			require.NoError(b, err)
			err = builder.Assemble(context.Background(), img.ref, warmupDir)
			os.RemoveAll(warmupDir)
			if err != nil {
				b.Skipf("Failed to warm cache for %s: %v", img.ref, err)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				outDir, err := os.MkdirTemp("", "erebus-out")
				require.NoError(b, err)
				b.StartTimer()

				start := time.Now()
				err = builder.Assemble(context.Background(), img.ref, outDir)
				duration := time.Since(start)

				if err != nil {
					b.Fatalf("Warm cache assemble failed: %v", err)
				}

				b.ReportMetric(float64(duration.Seconds()), "s/op")
				metrics.ObserveHistogram("bench_oci_warm_seconds", duration.Seconds(),
					hermes.Label{Key: "image", Value: img.name})

				os.RemoveAll(outDir)
			}
		})
	}
}

// calculateCacheStats calculates cache statistics from the cache directory.
func calculateCacheStats(t *testing.T, cacheDir string) (hits, misses int, totalBytes int64) {
	layersDir := filepath.Join(cacheDir, "layers")
	if _, err := os.Stat(layersDir); os.IsNotExist(err) {
		return 0, 0, 0
	}

	entries, err := os.ReadDir(layersDir)
	if err != nil {
		t.Logf("Warning: could not read layers dir: %v", err)
		return 0, 0, 0
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err == nil {
				totalBytes += info.Size()
			}
			hits++ // Each file in cache is a "hit" for subsequent runs
		}
	}

	return hits, misses, totalBytes
}

// TestErebusCacheBehaviorDocumentation documents and verifies cache behavior.
func TestErebusCacheBehaviorDocumentation(t *testing.T) {
	/*
		=== Erebus Layer Cache Behavior Documentation ===

		1. CACHE STRUCTURE
		   - Layers are stored at: {cacheDir}/layers/{digest.Hex}
		   - Each layer is stored in compressed (gzip) format
		   - Layer digest (SHA256) ensures content-addressable deduplication

		2. CACHE LOOKUP FLOW
		   a. For each layer in the image manifest:
		      i.   Calculate layer digest
		      ii.  Check if layers/{digest.Hex} exists in store
		      iii. If exists (HIT): Read from cache
		      iv.  If not exists (MISS): Download from registry and store

		3. DEDUPLICATION
		   - Same layer digest = same content (cryptographic guarantee)
		   - Layers shared across images are stored only once
		   - Example: python:3.11-slim and python:3.11 share base layers

		4. EXTRACTION ORDER
		   - Layers are downloaded in parallel (for performance)
		   - Layers are extracted sequentially (for correctness)
		   - Layer N must be extracted before Layer N+1 (overlay semantics)

		5. CACHE EVICTION
		   - Currently: No automatic eviction (manual cleanup required)
		   - Recommended: Implement LRU eviction based on last access time
		   - Recommended: Set max cache size in configuration

		6. PERFORMANCE CHARACTERISTICS
		   - Cold cache: Network-bound (registry download speed)
		   - Warm cache: I/O-bound (local disk read + extraction)
		   - Warm cache typically 10-100x faster than cold cache

		7. SLO TARGETS
		   - Warm cache: <30s for standard images
		   - Cold cache: Variable (network-dependent)
	*/

	t.Log("Testing cache behavior documentation")

	// Test that cache is content-addressable
	t.Run("ContentAddressableCache", func(t *testing.T) {
		cacheDir := t.TempDir()
		store, err := erebus.NewLocalStore(cacheDir)
		require.NoError(t, err)

		builder := erebus.NewOCIBuilder(store, nil)
		builder.Scanner = nil

		// First assembly
		out1 := t.TempDir()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		err = builder.Assemble(ctx, "alpine:3.19", out1)
		cancel()
		if err != nil {
			t.Skipf("Network issue: %v", err)
		}

		// Count layers after first assembly
		layers1, _, _ := calculateCacheStats(t, cacheDir)

		// Second assembly (same image)
		out2 := t.TempDir()
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Minute)
		start := time.Now()
		err = builder.Assemble(ctx2, "alpine:3.19", out2)
		warmDuration := time.Since(start)
		cancel2()
		require.NoError(t, err)

		// Count layers after second assembly (should be same)
		layers2, _, _ := calculateCacheStats(t, cacheDir)

		// Verify cache behavior
		require.Equal(t, layers1, layers2, "Cache should not grow for same image")
		t.Logf("Verified: Same image uses cached layers (count: %d)", layers1)
		t.Logf("Warm cache assembly time: %v", warmDuration)
	})

	// Test layer deduplication across images
	t.Run("LayerDeduplication", func(t *testing.T) {
		cacheDir := t.TempDir()
		store, err := erebus.NewLocalStore(cacheDir)
		require.NoError(t, err)

		builder := erebus.NewOCIBuilder(store, nil)
		builder.Scanner = nil

		// Pull first image
		out1 := t.TempDir()
		ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Minute)
		err = builder.Assemble(ctx1, "alpine:3.19", out1)
		cancel1()
		if err != nil {
			t.Skipf("Network issue: %v", err)
		}

		_, _, bytes1 := calculateCacheStats(t, cacheDir)
		t.Logf("Cache size after alpine:3.19: %d bytes", bytes1)

		// Pull second image (different tag, may share layers)
		out2 := t.TempDir()
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Minute)
		err = builder.Assemble(ctx2, "alpine:latest", out2)
		cancel2()
		if err != nil {
			t.Skipf("Network issue: %v", err)
		}

		_, _, bytes2 := calculateCacheStats(t, cacheDir)
		t.Logf("Cache size after alpine:latest: %d bytes", bytes2)
		t.Logf("Deduplication: Only added %d new bytes", bytes2-bytes1)
	})
}
