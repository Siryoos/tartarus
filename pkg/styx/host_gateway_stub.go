//go:build !linux
// +build !linux

package styx

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type hostGateway struct {
	bridgeName string
	bridgeCIDR netip.Prefix
}

// NewHostGateway creates a stub gateway for non-Linux platforms
func NewHostGateway(bridgeName string, cidr netip.Prefix) (Gateway, error) {
	return &hostGateway{
		bridgeName: bridgeName,
		bridgeCIDR: cidr,
	}, nil
}

func (g *hostGateway) Attach(ctx context.Context, sandboxID domain.SandboxID, contract *Contract) (string, netip.Addr, netip.Addr, netip.Prefix, error) {
	return "", netip.Addr{}, netip.Addr{}, netip.Prefix{}, fmt.Errorf("host gateway not supported on non-Linux platforms")
}

func (g *hostGateway) Detach(ctx context.Context, sandboxID domain.SandboxID) error {
	return fmt.Errorf("host gateway not supported on non-Linux platforms")
}
