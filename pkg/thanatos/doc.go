// Package thanatos implements graceful termination handling — the God of
// Death who oversees the peaceful end of every sandbox's life.
//
// Named after the personification of non-violent death, Thanatos manages
// the orderly shutdown sequence for sandboxes: signalling, draining,
// checkpointing, and resource cleanup. It ensures that no sandbox exits
// abruptly and that all data is safely persisted before the process ends.
//
// # Core Types
//
//   - [Handler]: The main termination orchestrator. Invoked by Hecatoncheir
//     when a sandbox needs to be terminated (TTL expiry, explicit kill,
//     policy violation).
//
//   - [Scheduler]: Tracks TTL expiry for active sandboxes and triggers
//     termination via [Handler] when deadlines are reached.
//
//   - [Policy]: Defines the termination policy for a sandbox (grace period,
//     checkpoint-on-exit, signal sequence).
//
//   - [Controller]: Drives the overall termination state machine and
//     integrates with Hypnos for checkpoint creation.
//
//   - [Exporter]: Publishes sandbox exit events and final resource usage
//     to the audit log and metrics sink.
//
// # Termination Flow
//
//  1. [Scheduler] detects TTL expiry or receives explicit terminate command.
//  2. [Handler.Terminate] sends SIGTERM to the sandbox process.
//  3. After the grace period, [Handler] sends SIGKILL if process still runs.
//  4. If checkpoint-on-exit is set, [Controller] calls Hypnos to snapshot.
//  5. Lethe overlay is cleaned up; Styx network rules are removed.
//  6. Hades status is updated to Succeeded/Failed.
//  7. [Exporter] publishes the exit event.
//
// # Known Technical Debt
//
//   - The checkpoint-on-exit path is implemented but requires a running
//     Hypnos instance. If Hypnos is unavailable, the checkpoint is silently
//     skipped rather than failing the termination cleanly.
//
//   - [Scheduler] uses a time.Ticker for TTL enforcement. For thousands of
//     concurrent sandboxes with different TTLs, a min-heap priority queue
//     would be more efficient than a linear scan per tick.
//
//   - [Exporter] writes exit events to the audit log but does not emit
//     Prometheus metrics for sandbox exit codes or termination reasons.
//     Operators lack visibility into failure rates by exit cause.
//
//   - The signal sequence (SIGTERM → wait → SIGKILL) is hardcoded.
//     Some workloads may need a custom signal (e.g., SIGHUP for reload).
//     A configurable signal list in [Policy] would address this.
package thanatos
