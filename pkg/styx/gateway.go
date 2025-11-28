package styx

import (
	"context"
	"net/netip"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// Contract defines the network oath for a sandbox.

type Contract struct {
	ID           string         `json:"id"`
	AllowedCIDRs []netip.Prefix `json:"allowed_cidrs"`
	DenyPrivate  bool           `json:"deny_private"`
	DenyMetadata bool           `json:"deny_metadata"`
}

// Gateway is Styx: configures TAP devices + firewall rules for each sandbox.

type Gateway interface {
	Attach(ctx context.Context, sandboxID domain.SandboxID, contract *Contract) (tapName string, ip netip.Addr, gateway netip.Addr, cidr netip.Prefix, err error)
	Detach(ctx context.Context, sandboxID domain.SandboxID) error
}
