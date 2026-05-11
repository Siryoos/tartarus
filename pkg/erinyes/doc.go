// Package erinyes implements active policy enforcement — the Furies that hunt
// down sandboxes which violate their resource contracts.
//
// Named after the three Furies (Alecto, Megaera, Tisiphone) who punished
// transgressors, this package continuously monitors running sandboxes and
// enforces CPU, memory, timeout, and network policies.
//
// # Core Types
//
//   - [Fury]: The main monitoring loop interface. Implementations periodically
//     poll sandbox metrics and invoke policy callbacks on violations.
//
//   - [NetworkStats]: Collects per-sandbox ingress/egress byte counters from
//     the kernel's netlink interface. Used by [PollFury] to feed network
//     traffic data to classifiers and violators.
//
//   - [PollFury]: A polling-based implementation of [Fury] that samples
//     resource usage at a fixed interval and calls registered handlers.
//
// # Basic Usage
//
//	fury := erinyes.NewPollFury(runtime, erinyes.Config{
//	    Interval:       5 * time.Second,
//	    CPUThreshold:   90.0,  // % of allotted
//	    MemThreshold:   95.0,
//	    NetworkEgress:  1 << 30, // 1 GiB/s
//	})
//	fury.OnViolation(func(id domain.SandboxID, reason string) {
//	    olympus.Terminate(ctx, id)
//	})
//	go fury.Run(ctx)
//
// # Known Technical Debt
//
//   - [NetworkStats] reads from the host's netlink socket. Inside
//     containerised deployments (Docker-in-Docker, K8s pods) the netlink
//     namespace may be the host's, causing incorrect per-sandbox accounting.
//     Per-namespace netlink connections are needed.
//
//   - CPU accounting is currently approximated from the runtime's process
//     list. Firecracker VMs report the host-side jailer process CPU, not
//     the guest vCPU time. True guest CPU metering requires Firecracker's
//     balloon device or a guest agent.
//
//   - There is no exponential back-off on consecutive violations. A sandbox
//     that oscillates around a threshold can generate a storm of violation
//     events. A dampening/debounce mechanism is needed.
//
//   - erinyes depends on [tartarus.SandboxRuntime] for status polling,
//     creating a circular concern when the runtime is the subject of
//     enforcement. Consider a read-only metrics interface to decouple them.
package erinyes
