//go:build linux

package nyx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// MockSnapshotMachine implements SnapshotMachine
type MockSnapshotMachine struct {
	CreateSnapshotFunc func(ctx context.Context, memFilePath, snapshotPath string, opts ...firecracker.CreateSnapshotOpt) error
	StopVMMFunc        func() error
}

func (m *MockSnapshotMachine) CreateSnapshot(ctx context.Context, memFilePath, snapshotPath string, opts ...firecracker.CreateSnapshotOpt) error {
	if m.CreateSnapshotFunc != nil {
		return m.CreateSnapshotFunc(ctx, memFilePath, snapshotPath, opts...)
	}
	// Default behavior: create empty files
	if err := os.WriteFile(memFilePath, []byte("mem"), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(snapshotPath, []byte("disk"), 0644); err != nil {
		return err
	}
	return nil
}

func (m *MockSnapshotMachine) StopVMM() error {
	if m.StopVMMFunc != nil {
		return m.StopVMMFunc()
	}
	return nil
}

func TestLocalManager_Prepare(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "nyx-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	storeDir := filepath.Join(tmpDir, "store")
	snapDir := filepath.Join(tmpDir, "snapshots")

	store, err := erebus.NewLocalStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}

	logger := hermes.NewSlogAdapter() // Uses stdout, which is fine for tests

	mgr, err := NewLocalManager(store, nil, snapDir, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Mock VM Launcher
	vmCreated := false
	mgr.vmLauncher = func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
		vmCreated = true
		return &MockSnapshotMachine{}, nil
	}

	tpl := &domain.TemplateSpec{
		ID:          "tpl-1",
		BaseImage:   "base.img",
		KernelImage: "kernel",
		Resources:   domain.ResourceSpec{Mem: 128, CPU: 1000},
	}

	// Test 1: Prepare new snapshot
	ctx := context.Background()
	snap, err := mgr.Prepare(ctx, tpl)
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	if !vmCreated {
		t.Error("Expected VM to be created")
	}
	if snap == nil {
		t.Fatal("Expected snapshot to be returned")
	}
	if snap.Template != tpl.ID {
		t.Errorf("Expected template ID %s, got %s", tpl.ID, snap.Template)
	}

	// Verify files in store
	// Path convention: snapshots/<tplID>/<snapID>.mem
	// snap.Path is snapshots/<tplID>/<snapID>
	memKey := snap.Path + ".mem"
	diskKey := snap.Path + ".disk"

	exists, err := store.Exists(ctx, memKey)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("Expected mem file %s to exist in store", memKey)
	}

	exists, err = store.Exists(ctx, diskKey)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("Expected disk file %s to exist in store", diskKey)
	}

	// Test 2: Prepare existing snapshot (should reuse)
	vmCreated = false // Reset
	snap2, err := mgr.Prepare(ctx, tpl)
	if err != nil {
		t.Fatalf("Prepare 2 failed: %v", err)
	}

	if vmCreated {
		t.Error("Expected VM NOT to be created on second call")
	}
	if snap2.ID != snap.ID {
		t.Errorf("Expected same snapshot ID, got %s vs %s", snap2.ID, snap.ID)
	}
}

func TestLocalManager_GetSnapshot(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "nyx-test-get-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := erebus.NewLocalStore(filepath.Join(tmpDir, "store"))
	mgr, _ := NewLocalManager(store, nil, filepath.Join(tmpDir, "snapshots"), hermes.NewSlogAdapter())

	// Mock VM Launcher
	mgr.vmLauncher = func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
		return &MockSnapshotMachine{}, nil
	}

	tplID := domain.TemplateID("tpl-get")
	tpl := &domain.TemplateSpec{ID: tplID}

	// Prepare
	snap, err := mgr.Prepare(context.Background(), tpl)
	if err != nil {
		t.Fatal(err)
	}

	// Get
	got, err := mgr.GetSnapshot(context.Background(), tplID)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if got.ID != snap.ID {
		t.Errorf("Expected snapshot ID %s, got %s", snap.ID, got.ID)
	}

	// Get non-existent
	_, err = mgr.GetSnapshot(context.Background(), "invalid")
	if err == nil {
		t.Error("Expected error for non-existent template")
	}
}

func TestLocalManager_ListSnapshots(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "nyx-test-list-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := erebus.NewLocalStore(filepath.Join(tmpDir, "store"))
	mgr, _ := NewLocalManager(store, nil, filepath.Join(tmpDir, "snapshots"), hermes.NewSlogAdapter())

	mgr.vmLauncher = func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
		return &MockSnapshotMachine{}, nil
	}

	tplID := domain.TemplateID("tpl-list")
	tpl := &domain.TemplateSpec{ID: tplID}

	// Prepare
	_, err = mgr.Prepare(context.Background(), tpl)
	if err != nil {
		t.Fatal(err)
	}

	// List
	list, err := mgr.ListSnapshots(context.Background(), tplID)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 snapshot, got %d", len(list))
	}

	// List empty
	list, err = mgr.ListSnapshots(context.Background(), "empty")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("Expected 0 snapshots, got %d", len(list))
	}
}

func TestLocalManager_Invalidate(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "nyx-test-inv-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := erebus.NewLocalStore(filepath.Join(tmpDir, "store"))
	mgr, _ := NewLocalManager(store, nil, filepath.Join(tmpDir, "snapshots"), hermes.NewSlogAdapter())

	mgr.vmLauncher = func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
		return &MockSnapshotMachine{}, nil
	}

	tplID := domain.TemplateID("tpl-inv")
	tpl := &domain.TemplateSpec{ID: tplID}

	// Prepare
	_, err = mgr.Prepare(context.Background(), tpl)
	if err != nil {
		t.Fatal(err)
	}

	// Invalidate
	if err := mgr.Invalidate(context.Background(), tplID); err != nil {
		t.Fatal(err)
	}

	// Verify list is empty
	list, _ := mgr.ListSnapshots(context.Background(), tplID)
	if len(list) != 0 {
		t.Error("Expected snapshots to be cleared")
	}
}

func TestLocalManager_GetSnapshot_ReadThrough(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "nyx-test-rt-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	storeDir := filepath.Join(tmpDir, "store")
	snapDir1 := filepath.Join(tmpDir, "snapshots1")
	snapDir2 := filepath.Join(tmpDir, "snapshots2")

	// Shared store
	store, err := erebus.NewLocalStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}

	logger := hermes.NewSlogAdapter()

	// Manager 1: Prepares the snapshot
	mgr1, err := NewLocalManager(store, nil, snapDir1, logger)
	if err != nil {
		t.Fatal(err)
	}
	mgr1.vmLauncher = func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
		return &MockSnapshotMachine{}, nil
	}

	tplID := domain.TemplateID("tpl-rt")
	tpl := &domain.TemplateSpec{ID: tplID}

	// Prepare snapshot with Manager 1
	snap, err := mgr1.Prepare(context.Background(), tpl)
	if err != nil {
		t.Fatalf("Manager 1 Prepare failed: %v", err)
	}

	// Manager 2: Should fetch from store
	mgr2, err := NewLocalManager(store, nil, snapDir2, logger)
	if err != nil {
		t.Fatal(err)
	}
	// No VM launcher needed for read-through, but set it just in case logic drifts
	mgr2.vmLauncher = func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
		return nil, os.ErrInvalid // Should not be called
	}

	// Get snapshot with Manager 2
	got, err := mgr2.GetSnapshot(context.Background(), tplID)
	if err != nil {
		t.Fatalf("Manager 2 GetSnapshot failed: %v", err)
	}

	if got.ID != snap.ID {
		t.Errorf("Expected snapshot ID %s, got %s", snap.ID, got.ID)
	}

	// Verify files exist in Manager 2's snapshot dir
	memPath := filepath.Join(snapDir2, "snapshots", string(tplID), string(snap.ID)+".mem")
	if _, err := os.Stat(memPath); err != nil {
		t.Errorf("Expected mem file to be downloaded to %s", memPath)
	}
}

func TestLocalManager_DeleteSnapshot(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "nyx-test-del-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	storeDir := filepath.Join(tmpDir, "store")
	snapDir := filepath.Join(tmpDir, "snapshots")

	store, err := erebus.NewLocalStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := NewLocalManager(store, nil, snapDir, hermes.NewSlogAdapter())
	if err != nil {
		t.Fatal(err)
	}

	mgr.vmLauncher = func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
		return &MockSnapshotMachine{}, nil
	}

	tplID := domain.TemplateID("tpl-del")
	tpl := &domain.TemplateSpec{ID: tplID}

	// Prepare snapshot
	snap, err := mgr.Prepare(context.Background(), tpl)
	if err != nil {
		t.Fatal(err)
	}

	// Verify it exists
	list, _ := mgr.ListSnapshots(context.Background(), tplID)
	if len(list) != 1 {
		t.Fatal("Expected snapshot to exist")
	}

	// Delete
	if err := mgr.DeleteSnapshot(context.Background(), tplID, snap.ID); err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	// Verify removed from list
	list, _ = mgr.ListSnapshots(context.Background(), tplID)
	if len(list) != 0 {
		t.Error("Expected snapshot to be removed from list")
	}

	// Verify removed from store
	memKey := fmt.Sprintf("snapshots/%s/%s.mem", tplID, snap.ID)
	exists, err := store.Exists(context.Background(), memKey)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("Expected mem file to be deleted from store")
	}
}
