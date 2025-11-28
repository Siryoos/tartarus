//go:build linux

package nyx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/google/uuid"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

type LocalManager struct {
	Store       erebus.Store
	SnapshotDir string
	Logger      hermes.Logger

	mu         sync.Mutex
	byTemplate map[domain.TemplateID][]*Snapshot

	// vmLauncher is the function used to create a paused VM.
	// It is exposed for testing purposes.
	vmLauncher func(ctx context.Context, tpl *domain.TemplateSpec, socketPath string) (SnapshotMachine, error)
}

// SnapshotMachine is an interface that abstracts firecracker.Machine for snapshotting.
type SnapshotMachine interface {
	CreateSnapshot(ctx context.Context, memFilePath, snapshotPath string, opts ...firecracker.CreateSnapshotOpt) error
	StopVMM() error
}

func NewLocalManager(store erebus.Store, snapshotDir string, logger hermes.Logger) (*LocalManager, error) {
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot dir: %w", err)
	}

	lm := &LocalManager{
		Store:       store,
		SnapshotDir: snapshotDir,
		Logger:      logger,
		byTemplate:  make(map[domain.TemplateID][]*Snapshot),
	}
	lm.vmLauncher = lm.createPausedVM
	return lm, nil
}

func (m *LocalManager) Prepare(ctx context.Context, tpl *domain.TemplateSpec) (*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we already have a snapshot
	if snaps, ok := m.byTemplate[tpl.ID]; ok && len(snaps) > 0 {
		// Return the most recent one (last in list, assuming append order)
		// Or sort? For now, just return the last one.
		return snaps[len(snaps)-1], nil
	}

	m.Logger.Info(ctx, "Preparing new snapshot for template", map[string]any{"template_id": tpl.ID})

	// Create a new snapshot
	snapID := domain.SnapshotID(uuid.New().String())

	// We need to launch a VM to snapshot it.
	// We'll use a temporary socket directory.
	socketDir, err := os.MkdirTemp("", "nyx-prepare-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp socket dir: %w", err)
	}
	defer os.RemoveAll(socketDir)

	socketPath := filepath.Join(socketDir, "firecracker.sock")

	// Define paths for the snapshot files
	// We will write them to a temp location first, then Put to Erebus?
	// Or directly to where Erebus expects them if it's a LocalStore?
	// The requirement says "Write these snapshot files into SnapshotDir using the erebus.Store abstraction".
	// Firecracker needs a file path to write to.
	// If Erebus is LocalStore, we can give it the final path?
	// But Erebus interface is Put(key, reader).
	// Firecracker writes directly to disk.
	// So we should let Firecracker write to a temp file, then stream it to Erebus.

	memFile := filepath.Join(socketDir, "snapshot.mem")
	diskFile := filepath.Join(socketDir, "snapshot.disk")

	// Launch VM
	machine, err := m.vmLauncher(ctx, tpl, socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create paused VM: %w", err)
	}
	defer machine.StopVMM() // Cleanup

	// Create Snapshot
	// Firecracker SDK CreateSnapshot signature: CreateSnapshot(ctx, memFilePath, snapshotPath, opts...)
	if err := machine.CreateSnapshot(ctx, memFile, diskFile); err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Persist to Erebus
	memKey := fmt.Sprintf("snapshots/%s/%s.mem", tpl.ID, snapID)
	diskKey := fmt.Sprintf("snapshots/%s/%s.disk", tpl.ID, snapID)

	if err := m.uploadFile(ctx, memKey, memFile); err != nil {
		return nil, err
	}
	if err := m.uploadFile(ctx, diskKey, diskFile); err != nil {
		return nil, err
	}

	// Create Snapshot object
	// Path convention: snapshots/<templateID>/<snapshotID>
	// The runtime will append .mem and .disk
	basePath := fmt.Sprintf("snapshots/%s/%s", tpl.ID, snapID)

	snap := &Snapshot{
		ID:        snapID,
		Template:  tpl.ID,
		Path:      basePath,
		CreatedAt: time.Now(),
	}

	// Update cache
	m.byTemplate[tpl.ID] = append(m.byTemplate[tpl.ID], snap)

	return snap, nil
}

func (m *LocalManager) GetSnapshot(ctx context.Context, tplID domain.TemplateID) (*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	snaps, ok := m.byTemplate[tplID]
	if !ok || len(snaps) == 0 {
		// Auto-prepare?
		// We need the TemplateSpec to prepare.
		// If we don't have it, we can't auto-prepare unless we fetch it from somewhere.
		// The interface doesn't give us the spec here.
		// So we can't auto-prepare in GetSnapshot unless we have access to a Template registry.
		// For now, return error or empty?
		// Requirement says: "If none exist: Option 1: call Prepare and return its result."
		// But Prepare needs *domain.TemplateSpec. GetSnapshot only has tplID.
		// I will return an error for now, as I don't have the spec.
		return nil, fmt.Errorf("no snapshot found for template %s", tplID)
	}

	// Return the most recent
	return snaps[len(snaps)-1], nil
}

func (m *LocalManager) ListSnapshots(ctx context.Context, tplID domain.TemplateID) ([]*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	snaps := m.byTemplate[tplID]
	// Return a copy
	result := make([]*Snapshot, len(snaps))
	copy(result, snaps)
	return result, nil
}

func (m *LocalManager) Invalidate(ctx context.Context, tplID domain.TemplateID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.byTemplate, tplID)
	// TODO: Delete files from Erebus
	return nil
}

// Helper to upload file to Erebus
func (m *LocalManager) uploadFile(ctx context.Context, key string, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file %s: %w", path, err)
	}
	defer f.Close()

	if err := m.Store.Put(ctx, key, f); err != nil {
		return fmt.Errorf("failed to upload %s to erebus: %w", key, err)
	}
	return nil
}

// createPausedVM launches a VM and pauses it.
func (m *LocalManager) createPausedVM(ctx context.Context, tpl *domain.TemplateSpec, socketPath string) (SnapshotMachine, error) {
	// Basic configuration similar to FirecrackerRuntime
	// We need kernel image and rootfs.
	// tpl has KernelImage and BaseImage.

	// Convert resources
	memSz := int64(tpl.Resources.Mem)
	if memSz == 0 {
		memSz = 128
	}
	cpuCount := int64(tpl.Resources.CPU) / 1000
	if cpuCount < 1 {
		cpuCount = 1
	}

	fcCfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: tpl.KernelImage,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(cpuCount),
			MemSizeMib: firecracker.Int64(memSz),
			Smt:        firecracker.Bool(false),
		},
		Drives: []models.Drive{
			{
				DriveID:      firecracker.String("rootfs"),
				PathOnHost:   firecracker.String(tpl.BaseImage),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false), // Needs to be writable for boot?
			},
		},
		// No network needed for snapshot preparation usually, unless warmup requires it.
		// For now, no network.
	}

	// Command builder
	cmd := firecracker.VMCommandBuilder{}.
		WithSocketPath(socketPath).
		Build(ctx)

	machine, err := firecracker.NewMachine(ctx, fcCfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		return nil, fmt.Errorf("failed to create machine: %w", err)
	}

	if err := machine.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start machine: %w", err)
	}

	// Pause the VM
	if err := machine.PauseVM(ctx); err != nil {
		machine.StopVMM()
		return nil, fmt.Errorf("failed to pause VM: %w", err)
	}

	return machine, nil
}
