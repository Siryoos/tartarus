//go:build linux

package nyx

import (
	"context"
	"os"
	"testing"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

func TestPrepare_Warmup(t *testing.T) {
	store, _ := erebus.NewLocalStore(t.TempDir())
	logger := hermes.NewSlogAdapter()
	lm, _ := NewLocalManager(store, nil, t.TempDir(), logger)

	// Mock vmLauncher
	lm.vmLauncher = func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
		assert.NotEmpty(t, tpl.WarmupCommand)
		assert.Equal(t, []string{"echo", "warm"}, tpl.WarmupCommand)
		return &MockWarmupSnapshotMachine{}, nil
	}

	tpl := &domain.TemplateSpec{
		ID:            "test-warmup",
		BaseImage:     "test.img",
		KernelImage:   "vmlinux",
		WarmupCommand: []string{"echo", "warm"},
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 128,
		},
	}

	_, err := lm.Prepare(context.Background(), tpl)
	assert.NoError(t, err)
}

type MockWarmupSnapshotMachine struct{}

func (m *MockWarmupSnapshotMachine) CreateSnapshot(ctx context.Context, memFilePath, snapshotPath string, opts ...firecracker.CreateSnapshotOpt) error {
	// Create dummy files
	os.WriteFile(memFilePath, []byte("mem"), 0644)
	os.WriteFile(snapshotPath, []byte("disk"), 0644)
	return nil
}

func (m *MockWarmupSnapshotMachine) StopVMM() error {
	return nil
}
