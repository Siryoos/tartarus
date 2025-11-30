//go:build linux

package nyx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/google/uuid"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"golang.org/x/sync/singleflight"
)

type LocalManager struct {
	Store       erebus.Store
	OCIBuilder  *erebus.OCIBuilder
	SnapshotDir string
	Logger      hermes.Logger

	mu         sync.Mutex
	byTemplate map[domain.TemplateID][]*Snapshot
	group      singleflight.Group

	// vmLauncher is the function used to create a paused VM.
	// It is exposed for testing purposes.
	vmLauncher func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error)
}

// SnapshotMachine is an interface that abstracts firecracker.Machine for snapshotting.
type SnapshotMachine interface {
	CreateSnapshot(ctx context.Context, memFilePath, snapshotPath string, opts ...firecracker.CreateSnapshotOpt) error
	StopVMM() error
}

func NewLocalManager(store erebus.Store, ociBuilder *erebus.OCIBuilder, snapshotDir string, logger hermes.Logger) (*LocalManager, error) {
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot dir: %w", err)
	}

	lm := &LocalManager{
		Store:       store,
		OCIBuilder:  ociBuilder,
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

	memFile := filepath.Join(socketDir, "snapshot.mem")
	diskFile := filepath.Join(socketDir, "snapshot.disk")

	// Determine rootfs path
	rootfsPath := tpl.BaseImage
	if strings.Contains(tpl.BaseImage, ":") || strings.Contains(tpl.BaseImage, "/") {
		// Assume OCI ref
		if m.OCIBuilder == nil {
			return nil, fmt.Errorf("OCI builder not configured but base image looks like OCI ref: %s", tpl.BaseImage)
		}

		// Extract to temp dir
		extractDir, err := os.MkdirTemp("", "oci-extract-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create extract dir: %w", err)
		}
		defer os.RemoveAll(extractDir)

		if err := m.OCIBuilder.Assemble(ctx, tpl.BaseImage, extractDir); err != nil {
			return nil, fmt.Errorf("failed to assemble OCI image: %w", err)
		}

		// Build rootfs image
		ociRootfs := filepath.Join(socketDir, "rootfs.img")
		if err := m.OCIBuilder.BuildRootFS(ctx, extractDir, ociRootfs); err != nil {
			return nil, fmt.Errorf("failed to build rootfs from OCI: %w", err)
		}
		rootfsPath = ociRootfs
	}

	// Launch VM
	machine, err := m.vmLauncher(ctx, tpl, rootfsPath, socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create paused VM: %w", err)
	}
	defer machine.StopVMM() // Cleanup

	// Create Snapshot
	if err := machine.CreateSnapshot(ctx, memFile, diskFile); err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Persist to Erebus
	memKey := fmt.Sprintf("snapshots/%s/%s.mem", tpl.ID, snapID)
	diskKey := fmt.Sprintf("snapshots/%s/%s.disk", tpl.ID, snapID)
	latestKey := fmt.Sprintf("snapshots/%s/latest", tpl.ID)

	if err := m.uploadFile(ctx, memKey, memFile); err != nil {
		return nil, err
	}
	if err := m.uploadFile(ctx, diskKey, diskFile); err != nil {
		return nil, err
	}
	// Update 'latest' pointer
	if err := m.Store.Put(ctx, latestKey, bytes.NewReader([]byte(snapID))); err != nil {
		return nil, fmt.Errorf("failed to update latest pointer: %w", err)
	}

	// Move files to local cache (SnapshotDir) so they are available for immediate use
	// The runtime expects them at SnapshotDir/snapshots/<tplID>/<snapID>.{mem,disk} usually,
	// or we can just use the paths we have.
	// But GetSnapshot logic below assumes they are in SnapshotDir.
	// So let's install them there.
	finalDir := filepath.Join(m.SnapshotDir, "snapshots", string(tpl.ID))
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot final dir: %w", err)
	}

	finalMemPath := filepath.Join(finalDir, string(snapID)+".mem")
	finalDiskPath := filepath.Join(finalDir, string(snapID)+".disk")

	if err := copyFile(memFile, finalMemPath); err != nil {
		return nil, fmt.Errorf("failed to cache mem file: %w", err)
	}
	if err := copyFile(diskFile, finalDiskPath); err != nil {
		return nil, fmt.Errorf("failed to cache disk file: %w", err)
	}

	// Create Snapshot object
	// Path convention: snapshots/<templateID>/<snapshotID>
	// The runtime will append .mem and .disk
	basePath := filepath.Join(m.SnapshotDir, "snapshots", string(tpl.ID), string(snapID))

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
	// 1. Check in-memory cache first
	m.mu.Lock()
	snaps, ok := m.byTemplate[tplID]
	if ok && len(snaps) > 0 {
		m.mu.Unlock()
		return snaps[len(snaps)-1], nil
	}
	m.mu.Unlock()

	// 2. Read-through from Erebus
	res, err, _ := m.group.Do(string(tplID), func() (interface{}, error) {
		// Double check memory
		m.mu.Lock()
		if snaps, ok := m.byTemplate[tplID]; ok && len(snaps) > 0 {
			m.mu.Unlock()
			return snaps[len(snaps)-1], nil
		}
		m.mu.Unlock()

		// Fetch 'latest' pointer
		latestKey := fmt.Sprintf("snapshots/%s/latest", tplID)
		r, err := m.Store.Get(ctx, latestKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get latest snapshot pointer: %w", err)
		}
		defer r.Close()

		snapIDBytes, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read latest snapshot pointer: %w", err)
		}
		snapID := domain.SnapshotID(string(snapIDBytes))

		// Check if we have this snapshot locally on disk (but not in memory)
		finalDir := filepath.Join(m.SnapshotDir, "snapshots", string(tplID))
		finalMemPath := filepath.Join(finalDir, string(snapID)+".mem")
		finalDiskPath := filepath.Join(finalDir, string(snapID)+".disk")
		basePath := filepath.Join(m.SnapshotDir, "snapshots", string(tplID), string(snapID))

		// If files exist, just load into memory
		if _, err := os.Stat(finalMemPath); err == nil {
			if _, err := os.Stat(finalDiskPath); err == nil {
				snap := &Snapshot{
					ID:        snapID,
					Template:  tplID,
					Path:      basePath,
					CreatedAt: time.Now(), // We don't know the real time, but that's okay
				}
				m.mu.Lock()
				m.byTemplate[tplID] = append(m.byTemplate[tplID], snap)
				m.mu.Unlock()
				return snap, nil
			}
		}

		// Download files
		if err := os.MkdirAll(finalDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create snapshot dir: %w", err)
		}

		memKey := fmt.Sprintf("snapshots/%s/%s.mem", tplID, snapID)
		diskKey := fmt.Sprintf("snapshots/%s/%s.disk", tplID, snapID)

		if err := m.downloadFile(ctx, memKey, finalMemPath); err != nil {
			return nil, err
		}
		if err := m.downloadFile(ctx, diskKey, finalDiskPath); err != nil {
			return nil, err
		}

		snap := &Snapshot{
			ID:        snapID,
			Template:  tplID,
			Path:      basePath,
			CreatedAt: time.Now(),
		}

		m.mu.Lock()
		m.byTemplate[tplID] = append(m.byTemplate[tplID], snap)
		m.mu.Unlock()

		return snap, nil
	})

	if err != nil {
		return nil, err
	}
	return res.(*Snapshot), nil
}

func (m *LocalManager) ListSnapshots(ctx context.Context, tplID domain.TemplateID) ([]*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	snaps := m.byTemplate[tplID]
	result := make([]*Snapshot, len(snaps))
	copy(result, snaps)
	return result, nil
}

func (m *LocalManager) Invalidate(ctx context.Context, tplID domain.TemplateID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.byTemplate, tplID)
	// TODO: Delete files from Erebus?
	return nil
}

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

func (m *LocalManager) downloadFile(ctx context.Context, key string, path string) error {
	r, err := m.Store.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get %s from erebus: %w", key, err)
	}
	defer r.Close()

	// Write to temp file first
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file %s: %w", tmpPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("failed to download %s: %w", key, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tmpPath, path, err)
	}
	return nil
}

func (m *LocalManager) createPausedVM(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
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
				PathOnHost:   firecracker.String(rootfsPath),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false),
			},
		},
	}

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

	if err := machine.PauseVM(ctx); err != nil {
		machine.StopVMM()
		return nil, fmt.Errorf("failed to pause VM: %w", err)
	}

	return machine, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
