// Package styx implements the network gateway — the River of Oaths that
// enforces binding network contracts for every sandbox.
//
// Named after the river by which the gods swore unbreakable oaths, Styx
// enforces network isolation rules for sandboxes. It applies iptables/nftables
// rules to restrict ingress and egress traffic, preventing sandboxes from
// communicating with hosts, services, or IP ranges that are not explicitly
// allowed by their [domain.SandboxRequest.NetworkRef] policy.
//
// # Core Types
//
//   - [Gateway]: Interface for applying and removing network policies for a
//     given sandbox ID.
//
//   - [HostGateway]: Linux-native implementation that manages network
//     namespaces and iptables rules for each sandbox.
//
//   - [HostGatewayStub]: No-op stub for non-Linux build targets and testing.
//
// # Network Policy
//
// Policies reference a [domain.NetworkPolicyRef] which is resolved against
// the TenantNetworkPolicy CRD (managed by the Kubernetes operator) or a
// static policy file. The resolved policy specifies:
//
//   - Allowed egress CIDR ranges and ports.
//   - Allowed ingress sources (other sandboxes, external IPs).
//   - Whether internet access is permitted.
//
// # Basic Usage
//
//	gw := styx.NewHostGateway(styx.Config{
//	    BridgeName:  "tartarus0",
//	    Subnet:      "10.100.0.0/16",
//	})
//
//	// Before launch:
//	if err := gw.Apply(ctx, sandboxID, networkPolicy); err != nil { ... }
//
//	// After termination:
//	gw.Remove(ctx, sandboxID)
//
// # Known Technical Debt
//
//   - [HostGateway] uses iptables via shell exec rather than the netlink API.
//     This is fragile (depends on iptables binary path, version), slow, and
//     incompatible with systems using nftables. A direct netlink or
//     nftables implementation is required.
//
//   - Network namespace management is manual. There is no integration with
//     CNI plugins, making Styx incompatible with standard Kubernetes
//     networking (Calico, Cilium, Flannel).
//
//   - Egress traffic shaping (bandwidth limits) is declared in the policy
//     model but not enforced. tc (traffic control) integration is needed.
//
//   - [HostGatewayStub] silently accepts all policy applications, which
//     means network policy tests on non-Linux CI systems are effectively
//     no-ops and provide no coverage guarantee.
package styx
