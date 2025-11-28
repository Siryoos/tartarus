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

func (r *FirecrackerRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer) error {
	return fmt.Errorf("Firecracker runtime not supported on non-Linux platforms")
}
