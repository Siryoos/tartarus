package erebus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Representative large images for benchmarking
var representativeImages = []struct {
	name string
	ref  string
}{
	{"Python", "python:3.11-slim"},
	{"Node", "node:18"},
	{"Ubuntu", "ubuntu:22.04"},
}

// BenchmarkOCIBuilder_Assemble_ColdCache benchmarks conversion with cold cache (first run)
func BenchmarkOCIBuilder_Assemble_ColdCache(b *testing.B) {
	for _, img := range representativeImages {
		b.Run(img.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// Create a fresh cache dir for cold cache simulation
				cacheDir, err := os.MkdirTemp("", "erebus-bench-cache-cold")
				require.NoError(b, err)

				store, err := NewLocalStore(cacheDir)
				require.NoError(b, err)

				builder := NewOCIBuilder(store, nil)
				builder.Scanner = nil // Disable scanner for benchmark

				outDir, err := os.MkdirTemp("", "erebus-bench-out")
				require.NoError(b, err)

				b.StartTimer()
				start := time.Now()
				err = builder.Assemble(context.Background(), img.ref, outDir)
				duration := time.Since(start)
				b.StopTimer()

				require.NoError(b, err)
				b.ReportMetric(float64(duration.Seconds()), "s/op")

				os.RemoveAll(outDir)
				os.RemoveAll(cacheDir)
			}
		})
	}
}

// BenchmarkOCIBuilder_Assemble_WarmCache benchmarks conversion with warm cache
func BenchmarkOCIBuilder_Assemble_WarmCache(b *testing.B) {
	for _, img := range representativeImages {
		b.Run(img.name, func(b *testing.B) {
			// Set up persistent cache across iterations
			cacheDir, err := os.MkdirTemp("", "erebus-bench-cache-warm")
			require.NoError(b, err)
			defer os.RemoveAll(cacheDir)

			store, err := NewLocalStore(cacheDir)
			require.NoError(b, err)

			builder := NewOCIBuilder(store, nil)
			builder.Scanner = nil

			// Warm up the cache with first run
			warmupDir, err := os.MkdirTemp("", "erebus-warmup")
			require.NoError(b, err)
			err = builder.Assemble(context.Background(), img.ref, warmupDir)
			require.NoError(b, err)
			os.RemoveAll(warmupDir)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				outDir, err := os.MkdirTemp("", "erebus-bench-out")
				require.NoError(b, err)

				start := time.Now()
				err = builder.Assemble(context.Background(), img.ref, outDir)
				duration := time.Since(start)

				require.NoError(b, err)
				b.ReportMetric(float64(duration.Seconds()), "s/op")

				os.RemoveAll(outDir)
			}
		})
	}
}

// BenchmarkOCIBuilder_BuildRootFS benchmarks the ext4 image creation stage
func BenchmarkOCIBuilder_BuildRootFS(b *testing.B) {
	for _, img := range representativeImages {
		b.Run(img.name, func(b *testing.B) {
			// Set up a pre-extracted rootfs directory
			cacheDir, err := os.MkdirTemp("", "erebus-bench-cache")
			require.NoError(b, err)
			defer os.RemoveAll(cacheDir)

			store, err := NewLocalStore(cacheDir)
			require.NoError(b, err)

			builder := NewOCIBuilder(store, nil)
			builder.Scanner = nil

			srcDir, err := os.MkdirTemp("", "erebus-bench-src")
			require.NoError(b, err)

			// Extract image once for reuse
			err = builder.Assemble(context.Background(), img.ref, srcDir)
			require.NoError(b, err)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				outDir, err := os.MkdirTemp("", "erebus-bench-rootfs")
				require.NoError(b, err)
				dstFile := filepath.Join(outDir, "rootfs.ext4")
				b.StartTimer()

				start := time.Now()
				err = builder.BuildRootFS(context.Background(), srcDir, dstFile)
				duration := time.Since(start)
				b.StopTimer()

				if err != nil && err != ErrToolNotFound {
					require.NoError(b, err)
				} else if err == ErrToolNotFound {
					b.Skip("genext2fs not found, skipping BuildRootFS benchmark")
				}

				b.ReportMetric(float64(duration.Seconds()), "s/op")
				os.RemoveAll(outDir)
			}

			os.RemoveAll(srcDir)
		})
	}
}
