// Package hecatoncheir implements the node agent — the Hundred-Handed Guardian
// that manages the full lifecycle of sandboxes on a single worker node.
//
// Named after the hundred-handed giants of Greek mythology who could fight on
// a hundred fronts simultaneously, Hecatoncheir dequeues sandbox requests
// from Acheron, launches them via the appropriate runtime, enforces resource
// policies, and reports status back to Hades and Olympus.
//
// # Core Types
//
//   - [Agent]: The main worker goroutine. It owns an [acheron.Queue] consumer,
//     a [tartarus.SandboxRuntime], and all supporting subsystems.
//
//   - [ControlListener]: Listens on a Redis Pub/Sub channel for control
//     commands (KILL, LOGS, HIBERNATE, WAKE, SNAPSHOT, EXEC, LIST_SANDBOXES)
//     issued by the Olympus control plane.
//
//   - [RedisControlListener]: Production implementation of [ControlListener]
//     using Redis Pub/Sub.
//
// # Sandbox Lifecycle on a Node
//
//  1. [Agent.Run] dequeues a [domain.SandboxRequest] from Acheron.
//  2. Judges evaluate the request for admission (quota, security).
//  3. Lethe provisions a clean copy-on-write filesystem overlay.
//  4. Styx applies network isolation rules.
//  5. The [tartarus.SandboxRuntime] launches the sandbox.
//  6. Hades is updated with the running [domain.SandboxRun].
//  7. Erinyes monitors the sandbox for policy violations.
//  8. On exit, Thanatos handles graceful termination and cleanup.
//
// # Control Plane Integration
//
// Control messages arrive on Redis topic "tartarus:control:<nodeID>".
// The agent serialises and deserialises [ControlMessage] structs and dispatches
// them to the appropriate handler (e.g., kill, stream logs, exec).
//
// # Known Technical Debt
//
//   - [Agent.Reconcile] replays in-flight runs on startup, but does not
//     handle the case where the runtime has lost state (e.g., after a host
//     reboot). Orphaned run records in Hades are never cleaned up.
//
//   - The exec path ([ControlMessageExec]) delegates to [tartarus.Exec] which
//     returns ErrNotImplemented for the Firecracker runtime. Firecracker exec
//     requires a guest agent (vsock-based) which is not yet implemented.
//
//   - Log streaming ([ControlMessageLogs]) reads from a local file. There is
//     no structured log forwarding to a centralised store. For long-running
//     sandboxes, the log file can grow without bound.
//
//   - The agent currently serialises dequeue and launch in a single goroutine.
//     A worker-pool model with configurable concurrency is needed to fully
//     utilise multi-core nodes.
//
//   - Snapshot creation ([ControlMessageSnapshot]) is delegated to Nyx but
//     there is no completion acknowledgement back to the control plane.
//     The caller must poll Nyx for snapshot status.
package hecatoncheir
