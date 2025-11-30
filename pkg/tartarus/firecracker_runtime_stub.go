//go:build !linux
// +build !linux

package tartarus

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// FirecrackerRuntime stub for non-Linux platforms
type FirecrackerRuntime struct {
	Logger *slog.Logger
}

func NewFirecrackerRuntime(logger *slog.Logger, socketDir, kernelImage, rootFSBase string) *FirecrackerRuntime {
	return &FirecrackerRuntime{Logger: logger}
}

func (r *FirecrackerRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error) {
	return nil, fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	return nil, fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	return fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) Pause(ctx context.Context, id domain.SandboxID) error {
	return fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) Resume(ctx context.Context, id domain.SandboxID) error {
	return fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	return fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error {
	return fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (VMConfig, *domain.SandboxRequest, error) {
	return VMConfig{}, nil, fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) {
	return nil, fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	return fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	return domain.ResourceCapacity{}, fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
	return fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}

func (r *FirecrackerRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string) error {
	return fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}
