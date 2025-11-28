package hypnos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// Manager implements basic sleep/hibernation on top of the runtime + Erebus.
// It snapshots a running sandbox, uploads the state, and can later wake it.
type Manager struct {
	Runtime    tartarus.SandboxRuntime
	Store      erebus.Store
	StagingDir string

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
	SandboxID   domain.SandboxID
	SnapshotKey string
	CreatedAt   time.Time
	Config      tartarus.VMConfig
	Request     domain.SandboxRequest
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
	if opts == nil {
		opts = &SleepOptions{GracefulShutdown: true}
	}

	cfg, req, err := m.Runtime.GetConfig(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sandbox config: %w", err)
	}
	if req == nil {
		return nil, fmt.Errorf("sandbox %s missing request metadata", id)
	}

	tmpDir, err := os.MkdirTemp(m.StagingDir, fmt.Sprintf("hypnos-%s-", id))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	snapshotBase := filepath.Join(tmpDir, "snapshot")
	memPath := snapshotBase + ".mem"
	diskPath := snapshotBase + ".disk"

	if err := m.Runtime.Pause(ctx, id); err != nil {
		return nil, fmt.Errorf("failed to pause sandbox: %w", err)
	}

	if err := m.Runtime.CreateSnapshot(ctx, id, memPath, diskPath); err != nil {
		// Best-effort resume if snapshotting fails.
		_ = m.Runtime.Resume(ctx, id)
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	if opts.GracefulShutdown {
		_ = m.Runtime.Shutdown(ctx, id)
	}
	// Ensure runtime state is cleared so we can re-launch on wake.
	_ = m.Runtime.Kill(ctx, id)

	keyBase := fmt.Sprintf("sleep/%s/%d", id, m.now().UnixNano())

	if err := m.copyToStore(ctx, keyBase+".mem", memPath); err != nil {
		return nil, err
	}
	if err := m.copyToStore(ctx, keyBase+".disk", diskPath); err != nil {
		return nil, err
	}

	record := &SleepRecord{
		SandboxID:   id,
		SnapshotKey: keyBase,
		CreatedAt:   m.now(),
		Config:      cfg,
		Request:     *req,
	}

	m.mu.Lock()
	m.sleeping[id] = record
	m.mu.Unlock()

	return record, nil
}

// Wake restores a previously sleeping sandbox.
func (m *Manager) Wake(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	record, ok := m.getRecord(id)
	if !ok {
		return nil, fmt.Errorf("sandbox %s is not sleeping", id)
	}

	tmpDir, err := os.MkdirTemp(m.StagingDir, fmt.Sprintf("hypnos-wake-%s-", id))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	snapshotBase := filepath.Join(tmpDir, "snapshot")
	memPath := snapshotBase + ".mem"
	diskPath := snapshotBase + ".disk"

	if err := m.copyFromStore(ctx, record.SnapshotKey+".mem", memPath); err != nil {
		return nil, err
	}
	if err := m.copyFromStore(ctx, record.SnapshotKey+".disk", diskPath); err != nil {
		return nil, err
	}

	cfg := record.Config
	cfg.Snapshot.Path = snapshotBase

	req := record.Request
	run, err := m.Runtime.Launch(ctx, &req, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to wake sandbox: %w", err)
	}

	m.mu.Lock()
	delete(m.sleeping, id)
	m.mu.Unlock()

	// Snapshot files are no longer needed once the VM is running.
	_ = os.RemoveAll(tmpDir)

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
