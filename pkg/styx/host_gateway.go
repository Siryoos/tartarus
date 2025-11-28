package styx

import (
	"context"
	"fmt"
	"net"
	"sync"

	"net/netip"

	"github.com/coreos/go-iptables/iptables"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

type hostGateway struct {
	bridgeName  string
	bridgeCIDR  netip.Prefix
	ipt         *iptables.IPTables
	mu          sync.Mutex
	allocations map[domain.SandboxID]netip.Addr
}

// NewHostGateway creates a new Gateway implementation for the host.
func NewHostGateway(bridgeName string, cidr netip.Prefix) (Gateway, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize iptables: %w", err)
	}

	return &hostGateway{
		bridgeName:  bridgeName,
		bridgeCIDR:  cidr,
		ipt:         ipt,
		allocations: make(map[domain.SandboxID]netip.Addr),
	}, nil
}

func (g *hostGateway) Attach(ctx context.Context, sandboxID domain.SandboxID, contract *Contract) (string, netip.Addr, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 1. Ensure Bridge Exists
	br, err := g.ensureBridge()
	if err != nil {
		return "", netip.Addr{}, fmt.Errorf("failed to ensure bridge %s: %w", g.bridgeName, err)
	}

	// 2. Allocate IP
	ip, err := g.allocateIP(sandboxID)
	if err != nil {
		return "", netip.Addr{}, fmt.Errorf("failed to allocate IP for sandbox %s: %w", sandboxID, err)
	}

	// 3. Create TAP
	tapName := fmt.Sprintf("tap-%s", string(sandboxID)[:8])
	tap, err := g.createTAP(tapName)
	if err != nil {
		// Rollback IP allocation
		delete(g.allocations, sandboxID)
		return "", netip.Addr{}, fmt.Errorf("failed to create TAP %s: %w", tapName, err)
	}

	// 4. Attach TAP to Bridge
	if err := netlink.LinkSetMaster(tap, br); err != nil {
		// Rollback
		_ = netlink.LinkDel(tap)
		delete(g.allocations, sandboxID)
		return "", netip.Addr{}, fmt.Errorf("failed to attach TAP %s to bridge %s: %w", tapName, g.bridgeName, err)
	}

	// 5. Configure iptables
	if err := g.ensureIptablesRules(); err != nil {
		// We don't strictly rollback here as rules are global/idempotent, but it's an error.
		// For strict correctness we might want to warn, but let's return error.
		return "", netip.Addr{}, fmt.Errorf("failed to configure iptables: %w", err)
	}

	return tapName, ip, nil
}

func (g *hostGateway) Detach(ctx context.Context, sandboxID domain.SandboxID) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 1. Find TAP and IP
	if _, ok := g.allocations[sandboxID]; !ok {
		// Already detached or never attached
		return nil
	}

	tapName := fmt.Sprintf("tap-%s", string(sandboxID)[:8])

	// 2. Delete TAP
	// We look it up by name
	link, err := netlink.LinkByName(tapName)
	if err == nil {
		// If it exists, delete it
		if err := netlink.LinkDel(link); err != nil {
			return fmt.Errorf("failed to delete TAP %s: %w", tapName, err)
		}
	} else {
		// If not found, maybe already deleted. Log or ignore?
		// We'll assume it's fine.
	}

	// 3. Free IP
	delete(g.allocations, sandboxID)

	return nil
}

func (g *hostGateway) ensureBridge() (*netlink.Bridge, error) {
	link, err := netlink.LinkByName(g.bridgeName)
	if err == nil {
		// Exists, check if it is a bridge
		br, ok := link.(*netlink.Bridge)
		if !ok {
			return nil, fmt.Errorf("link %s exists but is not a bridge", g.bridgeName)
		}
		// Ensure it is UP
		if err := netlink.LinkSetUp(br); err != nil {
			return nil, fmt.Errorf("failed to set bridge %s up: %w", g.bridgeName, err)
		}
		// We should also check/ensure IP address, but for simplicity we assume if it exists it's configured or we configure it.
		// Let's ensure IP is set.
		if err := g.ensureBridgeIP(br); err != nil {
			return nil, err
		}
		return br, nil
	}

	// Create it
	la := netlink.NewLinkAttrs()
	la.Name = g.bridgeName
	br := &netlink.Bridge{LinkAttrs: la}
	if err := netlink.LinkAdd(br); err != nil {
		return nil, fmt.Errorf("failed to create bridge %s: %w", g.bridgeName, err)
	}

	// Set UP
	if err := netlink.LinkSetUp(br); err != nil {
		return nil, fmt.Errorf("failed to set bridge %s up: %w", g.bridgeName, err)
	}

	// Assign IP
	if err := g.ensureBridgeIP(br); err != nil {
		return nil, err
	}

	return br, nil
}

func (g *hostGateway) ensureBridgeIP(br *netlink.Bridge) error {
	// Bridge IP is .1 of the CIDR
	// e.g. 10.200.0.0/24 -> 10.200.0.1/24

	addr := g.bridgeCIDR.Addr()
	// We want the first usable IP.
	// If CIDR is 10.200.0.0/24, we want 10.200.0.1
	// If the passed CIDR is already the address we want (e.g. user passed 10.200.0.1/24), we use it.
	// But usually CIDR implies network address.
	// Let's assume the constructor passed the Network Prefix.

	// Convert netip.Prefix to net.IPNet for netlink
	// We need to calculate the .1 address.

	// Quick hack: parse the string representation of the prefix, change last byte? No, that's unsafe.
	// Use netip.Addr methods.

	// Let's assume the bridge IP is the first IP in the subnet.
	// 10.200.0.0/24 -> 10.200.0.1

	// We need to iterate to get the next IP.
	bridgeIP := addr // Start with network address
	if !bridgeIP.Is4() {
		return fmt.Errorf("only IPv4 supported for now")
	}

	// Increment to .1
	// Note: netip.Addr is not mutable.
	// We can use Next().
	bridgeIP = bridgeIP.Next()

	// Construct the IPNet
	// netlink.Addr requires *net.IPNet

	ipNet := &net.IPNet{
		IP:   net.ParseIP(bridgeIP.String()),
		Mask: net.CIDRMask(g.bridgeCIDR.Bits(), 32),
	}

	nlAddr := &netlink.Addr{IPNet: ipNet}

	// Check if address already exists
	addrs, err := netlink.AddrList(br, nl.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("failed to list addresses for %s: %w", g.bridgeName, err)
	}

	for _, a := range addrs {
		if a.IPNet.String() == nlAddr.IPNet.String() {
			return nil // Already present
		}
	}

	if err := netlink.AddrAdd(br, nlAddr); err != nil {
		return fmt.Errorf("failed to add address %s to bridge %s: %w", nlAddr, g.bridgeName, err)
	}

	return nil
}

func (g *hostGateway) createTAP(name string) (*netlink.Tuntap, error) {
	// Check if exists
	if l, err := netlink.LinkByName(name); err == nil {
		// If it exists, we probably want to delete and recreate to be clean, or reuse.
		// For safety, let's delete and recreate.
		_ = netlink.LinkDel(l)
	}

	la := netlink.NewLinkAttrs()
	la.Name = name
	// Set MTU
	la.MTU = 1500

	tap := &netlink.Tuntap{
		LinkAttrs: la,
		Mode:      netlink.TUNTAP_MODE_TAP,
	}

	if err := netlink.LinkAdd(tap); err != nil {
		return nil, err
	}

	if err := netlink.LinkSetUp(tap); err != nil {
		_ = netlink.LinkDel(tap)
		return nil, err
	}

	return tap, nil
}

func (g *hostGateway) allocateIP(id domain.SandboxID) (netip.Addr, error) {
	// Simple allocator: iterate from .2 upwards
	// Check against g.allocations

	// We need to know which IPs are taken.
	used := make(map[netip.Addr]bool)
	for _, ip := range g.allocations {
		used[ip] = true
	}

	// Start from .2
	// Network address: g.bridgeCIDR.Addr()
	// Bridge IP: .1
	// First allocatable: .2

	current := g.bridgeCIDR.Addr().Next().Next() // .0 -> .1 -> .2

	// We need to iterate until we find a free one or run out of subnet.
	// How to check if we are still in subnet?
	// g.bridgeCIDR.Contains(current)

	for g.bridgeCIDR.Contains(current) {
		if !used[current] {
			g.allocations[id] = current
			return current, nil
		}
		current = current.Next()
	}

	return netip.Addr{}, fmt.Errorf("IP pool exhausted in %s", g.bridgeCIDR)
}

func (g *hostGateway) ensureIptablesRules() error {
	// 1. MASQUERADE
	// iptables -t nat -A POSTROUTING -s <bridgeCIDR> ! -d <bridgeCIDR> -j MASQUERADE
	// Actually, simpler: -s <bridgeCIDR> -j MASQUERADE is usually enough for outbound.
	// The prompt says: "Enable MASQUERADE for traffic originating from the bridge subnet going out of the default interface"
	// We can just say -s CIDR -j MASQUERADE.

	cidrStr := g.bridgeCIDR.String()

	exists, err := g.ipt.Exists("nat", "POSTROUTING", "-s", cidrStr, "-j", "MASQUERADE")
	if err != nil {
		return err
	}
	if !exists {
		if err := g.ipt.Append("nat", "POSTROUTING", "-s", cidrStr, "-j", "MASQUERADE"); err != nil {
			return err
		}
	}

	// 2. FORWARD
	// Allow forwarding between br0 and host.
	// iptables -A FORWARD -i br0 -j ACCEPT
	// iptables -A FORWARD -o br0 -j ACCEPT

	// In:
	exists, err = g.ipt.Exists("filter", "FORWARD", "-i", g.bridgeName, "-j", "ACCEPT")
	if err != nil {
		return err
	}
	if !exists {
		if err := g.ipt.Append("filter", "FORWARD", "-i", g.bridgeName, "-j", "ACCEPT"); err != nil {
			return err
		}
	}

	// Out:
	exists, err = g.ipt.Exists("filter", "FORWARD", "-o", g.bridgeName, "-j", "ACCEPT")
	if err != nil {
		return err
	}
	if !exists {
		if err := g.ipt.Append("filter", "FORWARD", "-o", g.bridgeName, "-j", "ACCEPT"); err != nil {
			return err
		}
	}

	return nil
}
