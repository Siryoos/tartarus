package erinyes

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vishvananda/netlink"
)

// NetworkStatsProvider defines the interface for gathering network statistics.
type NetworkStatsProvider interface {
	// GetInterfaceStats returns RX/TX bytes for a given interface.
	// Note: Host RX = VM Egress (VM sends to host).
	//       Host TX = VM Ingress (Host sends to VM).
	GetInterfaceStats(ctx context.Context, ifaceName string) (rxBytes, txBytes int64, err error)

	// GetDropCount returns the number of packets dropped by iptables for the given TAP interface.
	GetDropCount(ctx context.Context, tapName string) (int, error)
}

// LinuxNetworkStatsProvider implements NetworkStatsProvider using netlink and iptables.
type LinuxNetworkStatsProvider struct{}

// NewLinuxNetworkStatsProvider creates a new LinuxNetworkStatsProvider.
func NewLinuxNetworkStatsProvider() *LinuxNetworkStatsProvider {
	return &LinuxNetworkStatsProvider{}
}

func (p *LinuxNetworkStatsProvider) GetInterfaceStats(ctx context.Context, ifaceName string) (int64, int64, error) {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get link %s: %w", ifaceName, err)
	}

	attrs := link.Attrs()
	if attrs == nil || attrs.Statistics == nil {
		return 0, 0, fmt.Errorf("no statistics available for link %s", ifaceName)
	}

	// Host RX is traffic coming FROM the VM (VM Egress)
	// Host TX is traffic going TO the VM (VM Ingress)
	return int64(attrs.Statistics.RxBytes), int64(attrs.Statistics.TxBytes), nil
}

func (p *LinuxNetworkStatsProvider) GetDropCount(ctx context.Context, tapName string) (int, error) {
	// We need to query iptables to see how many packets were dropped for this interface.
	// We assume Styx adds rules like:
	// -A FORWARD -i tap-xxxx -d 169.254.169.254/32 -j DROP
	// -A FORWARD -i tap-xxxx -d 10.0.0.0/8 -j DROP
	// etc.
	// We want to sum up the packet counters for all DROP rules matching this interface.

	// Command: iptables -v -x -n -L FORWARD
	// Output format:
	// pkts bytes target     prot opt in     out     source               destination
	// 0    0     DROP       all  --  tap-xxx *       0.0.0.0/0            169.254.169.254

	cmd := exec.CommandContext(ctx, "iptables", "-v", "-x", "-n", "-L", "FORWARD")
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run iptables: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	totalDrops := 0

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		// Fields:
		// 0: pkts
		// 1: bytes
		// 2: target
		// ...
		// 5: in interface
		// ...

		target := fields[2]
		inIface := fields[5]

		if target == "DROP" && inIface == tapName {
			pkts, err := strconv.Atoi(fields[0])
			if err == nil {
				totalDrops += pkts
			}
		}
	}

	return totalDrops, nil
}
