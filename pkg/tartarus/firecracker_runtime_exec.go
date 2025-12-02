//go:build linux
// +build linux

package tartarus

import (
	"context"
	"fmt"
	"io"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func (r *FirecrackerRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	// Firecracker does not support exec without a guest agent.
	// We return ErrNotImplemented for now.
	return fmt.Errorf("exec not implemented for Firecracker runtime")
}

func (r *FirecrackerRuntime) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return fmt.Errorf("exec interactive not implemented for Firecracker runtime")
}
