//go:build linux
// +build linux

package tartarus

import (
	"context"
	"fmt"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func (r *FirecrackerRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string) error {
	// Firecracker does not support exec without a guest agent.
	// We return ErrNotImplemented for now.
	return fmt.Errorf("exec not implemented for Firecracker runtime")
}
