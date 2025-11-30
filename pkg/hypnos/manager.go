package hypnos

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// LifecycleHooks allows external components to react to hibernation lifecycle events.
type LifecycleHooks struct {
	// PreSleep is called before the sandbox is hibernated.
	PreSleep func(context.Context, domain.SandboxID) error
	// PostSleep is called after the sandbox has been successfully hibernated.
	PostSleep func(context.Context, domain.SandboxID, *SleepRecord) error
	// PreWake is called before waking a hibernated sandbox.
	PreWake func(context.Context, domain.SandboxID, *SleepRecord) error
	// PostWake is called after a sandbox has been successfully woken.
	PostWake func(context.Context, domain.SandboxID, *domain.SandboxRun) error
}

// Manager implements sleep/hibernation on top of the runtime + Erebus.
// It snapshots a running sandbox, compresses it, uploads the state, and can later wake it.
type Manager struct {
	Runtime    tartarus.SandboxRuntime
	Store      erebus.Store
	StagingDir string
	Hooks      *LifecycleHooks
	Metrics    hermes.Metrics

	mu       sync.Mutex
	sleeping map[domain.SandboxID]*SleepRecord
	now      func() time.Time
}

// SleepOptions control how the sandbox is put to sleep.
type SleepOptions struct {
	// GracefulShutdown requests a clean shutdown after the snapshot is captured.
	GracefulShutdown bool
}

// SleepRecord tracks a hibernated sandbox.
type SleepRecord struct {
	SandboxID        domain.SandboxID
	SnapshotKey      string
	CreatedAt        time.Time
	Config           tartarus.VMConfig
	Request          domain.SandboxRequest
	CompressionRatio float64 // Ratio of compressed to uncompressed size
}

// NewManager constructs a Hypnos manager.
func NewManager(runtime tartarus.SandboxRuntime, store erebus.Store, stagingDir string) *Manager {
	if stagingDir == "" {
		stagingDir = os.TempDir()
	}
	return &Manager{
		Runtime:    runtime,
		Store:      store,
		StagingDir: stagingDir,
		sleeping:   make(map[domain.SandboxID]*SleepRecord),
		now:        time.Now,
	}
}

// Sleep captures a snapshot of the running sandbox and tears down the VM process.
func (m *Manager) Sleep(ctx context.Context, id domain.SandboxID, opts *SleepOptions) (*SleepRecord, error) {
	start := time.Now()
	if opts == nil {
		opts = &SleepOptions{GracefulShutdown: true}
	}

	// PreSleep hook
	if m.Hooks != nil && m.Hooks.PreSleep != nil {
		if err := m.Hooks.PreSleep(ctx, id); err != nil {
			if m.Metrics != nil {
				m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "pre_sleep_hook"})
			}
			return nil, fmt.Errorf("pre-sleep hook failed: %w", err)
		}
	}

	cfg, req, err := m.Runtime.GetConfig(ctx, id)
	if err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "get_config"})
		}
		return nil, fmt.Errorf("failed to fetch sandbox config: %w", err)
	}
	if req == nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "missing_request"})
		}
		return nil, fmt.Errorf("sandbox %s missing request metadata", id)
	}

	tmpDir, err := os.MkdirTemp(m.StagingDir, fmt.Sprintf("hypnos-%s-", id))
	if err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "create_temp_dir"})
		}
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	snapshotBase := filepath.Join(tmpDir, "snapshot")
	memPath := snapshotBase + ".mem"
	diskPath := snapshotBase + ".disk"

	if err := m.Runtime.Pause(ctx, id); err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "pause"})
		}
		return nil, fmt.Errorf("failed to pause sandbox: %w", err)
	}

	if err := m.Runtime.CreateSnapshot(ctx, id, memPath, diskPath); err != nil {
		// Best-effort resume if snapshotting fails.
		_ = m.Runtime.Resume(ctx, id)
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "create_snapshot"})
		}
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	if opts.GracefulShutdown {
		_ = m.Runtime.Shutdown(ctx, id)
	}
	// Ensure runtime state is cleared so we can re-launch on wake.
	_ = m.Runtime.Kill(ctx, id)

	keyBase := fmt.Sprintf("sleep/%s/%d", id, m.now().UnixNano())

	// Compress and upload memory snapshot
	memCompressedPath := memPath + ".gz"
	compressionRatio, err := m.compressFile(memPath, memCompressedPath)
	if err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "compress_memory"})
		}
		return nil, fmt.Errorf("failed to compress memory snapshot: %w", err)
	}

	if m.Metrics != nil {
		m.Metrics.ObserveHistogram("hypnos_compression_ratio", compressionRatio)
	}

	if err := m.copyToStore(ctx, keyBase+".mem.gz", memCompressedPath); err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "upload_memory"})
		}
		return nil, err
	}
	if err := m.copyToStore(ctx, keyBase+".disk", diskPath); err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "upload_disk"})
		}
		return nil, err
	}

	record := &SleepRecord{
		SandboxID:        id,
		SnapshotKey:      keyBase,
		CreatedAt:        m.now(),
		Config:           cfg,
		Request:          *req,
		CompressionRatio: compressionRatio,
	}

	m.mu.Lock()
	m.sleeping[id] = record
	m.mu.Unlock()

	// PostSleep hook
	if m.Hooks != nil && m.Hooks.PostSleep != nil {
		if err := m.Hooks.PostSleep(ctx, id, record); err != nil {
			// Don't fail the sleep operation, just log
			if m.Metrics != nil {
				m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "post_sleep_hook"})
			}
		}
	}

	// Track metrics
	if m.Metrics != nil {
		m.Metrics.IncCounter("hypnos_sleep_total", 1)
		m.Metrics.ObserveHistogram("hypnos_sleep_duration_seconds", time.Since(start).Seconds())
	}

	return record, nil
}

// Wake restores a previously sleeping sandbox.
func (m *Manager) Wake(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	start := time.Now()
	record, ok := m.getRecord(id)
	if !ok {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "not_sleeping"})
		}
		return nil, fmt.Errorf("sandbox %s is not sleeping", id)
	}

	// PreWake hook
	if m.Hooks != nil && m.Hooks.PreWake != nil {
		if err := m.Hooks.PreWake(ctx, id, record); err != nil {
			if m.Metrics != nil {
				m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "pre_wake_hook"})
			}
			return nil, fmt.Errorf("pre-wake hook failed: %w", err)
		}
	}

	tmpDir, err := os.MkdirTemp(m.StagingDir, fmt.Sprintf("hypnos-wake-%s-", id))
	if err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "wake_temp_dir"})
		}
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	snapshotBase := filepath.Join(tmpDir, "snapshot")
	memPath := snapshotBase + ".mem"
	memCompressedPath := memPath + ".gz"
	diskPath := snapshotBase + ".disk"

	// Download and decompress memory snapshot
	if err := m.copyFromStore(ctx, record.SnapshotKey+".mem.gz", memCompressedPath); err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "download_memory"})
		}
		return nil, err
	}

	if err := m.decompressFile(memCompressedPath, memPath); err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "decompress_memory"})
		}
		return nil, fmt.Errorf("failed to decompress memory snapshot: %w", err)
	}

	if err := m.copyFromStore(ctx, record.SnapshotKey+".disk", diskPath); err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "download_disk"})
		}
		return nil, err
	}

	cfg := record.Config
	cfg.Snapshot.Path = snapshotBase

	req := record.Request
	run, err := m.Runtime.Launch(ctx, &req, cfg)
	if err != nil {
		if m.Metrics != nil {
			m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "wake_launch"})
		}
		return nil, fmt.Errorf("failed to wake sandbox: %w", err)
	}

	m.mu.Lock()
	delete(m.sleeping, id)
	m.mu.Unlock()

	// Snapshot files are no longer needed once the VM is running.
	_ = os.RemoveAll(tmpDir)

	// PostWake hook
	if m.Hooks != nil && m.Hooks.PostWake != nil {
		if err := m.Hooks.PostWake(ctx, id, run); err != nil {
			// Don't fail the wake operation, just log
			if m.Metrics != nil {
				m.Metrics.IncCounter("hypnos_errors_total", 1, hermes.Label{Key: "phase", Value: "post_wake_hook"})
			}
		}
	}

	// Track metrics
	if m.Metrics != nil {
		m.Metrics.IncCounter("hypnos_wake_total", 1)
		m.Metrics.ObserveHistogram("hypnos_wake_duration_seconds", time.Since(start).Seconds())
	}

	return run, nil
}

// List returns all sleeping sandboxes.
func (m *Manager) List() []*SleepRecord {
	m.mu.Lock()
	defer m.mu.Unlock()

	records := make([]*SleepRecord, 0, len(m.sleeping))
	for _, rec := range m.sleeping {
		records = append(records, rec)
	}
	return records
}

// IsSleeping reports whether the sandbox is currently hibernating.
func (m *Manager) IsSleeping(id domain.SandboxID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sleeping[id]
	return ok
}

func (m *Manager) getRecord(id domain.SandboxID) (*SleepRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.sleeping[id]
	return rec, ok
}

func (m *Manager) copyToStore(ctx context.Context, key, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open snapshot %s: %w", path, err)
	}
	defer f.Close()

	if err := m.Store.Put(ctx, key, f); err != nil {
		return fmt.Errorf("failed to store %s: %w", key, err)
	}
	return nil
}

func (m *Manager) copyFromStore(ctx context.Context, key, path string) error {
	reader, err := m.Store.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to fetch %s: %w", key, err)
	}
	defer reader.Close()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create path for %s: %w", path, err)
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("failed to create temp file %s: %w", tmp, err)
	}
	if _, err := f.ReadFrom(reader); err != nil {
		f.Close()
		return fmt.Errorf("failed to copy snapshot %s: %w", key, err)
	}
	f.Close()

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to finalize snapshot %s: %w", key, err)
	}
	return nil
}

// compressFile compresses src to dst using gzip and returns the compression ratio.
func (m *Manager) compressFile(src, dst string) (float64, error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to stat source file: %w", err)
	}
	originalSize := srcInfo.Size()

	dstFile, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()

	written, err := io.Copy(gzWriter, srcFile)
	if err != nil {
		return 0, fmt.Errorf("failed to compress file: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return 0, fmt.Errorf("failed to finalize compression: %w", err)
	}

	dstInfo, err := dstFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to stat compressed file: %w", err)
	}
	compressedSize := dstInfo.Size()

	ratio := float64(compressedSize) / float64(originalSize)
	if originalSize == 0 {
		ratio = 0
	}

	_ = written // Silence unused variable
	return ratio, nil
}

// decompressFile decompresses src (gzipped) to dst.
func (m *Manager) decompressFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open compressed file: %w", err)
	}
	defer srcFile.Close()

	gzReader, err := gzip.NewReader(srcFile)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, gzReader); err != nil {
		return fmt.Errorf("failed to decompress file: %w", err)
	}

	return nil
}
