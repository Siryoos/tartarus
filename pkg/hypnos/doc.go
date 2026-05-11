// Package hypnos implements sleep and hibernation management — the God of
// Sleep who suspends sandboxes and restores them on demand.
//
// Named after the personification of sleep, Hypnos manages the lifecycle of
// sandbox hibernation: serialising VM memory state to a snapshot, freeing
// host resources, and transparently resuming execution when the sandbox is
// next needed.
//
// # Core Types
//
//   - [Manager]: Orchestrates hibernate and wake operations. It delegates
//     to the [nyx.Manager] for snapshot creation and restoration, and to
//     the [tartarus.SandboxRuntime] for pausing/resuming execution.
//
// # Hibernation Flow
//
//  1. Caller invokes [Manager.Hibernate] with a sandbox ID.
//  2. Hypnos signals the runtime to pause the sandbox.
//  3. The VM memory snapshot is written to Nyx storage.
//  4. The sandbox process is terminated; host CPU/memory is released.
//  5. Hades status is updated to Hibernated.
//
// # Wake Flow
//
//  1. Caller invokes [Manager.Wake] with a sandbox ID.
//  2. Hypnos retrieves the snapshot from Nyx.
//  3. The runtime restores the VM from the snapshot.
//  4. Hades status is updated to Running.
//
// # Known Technical Debt
//
//   - Hibernation latency is dominated by the time to serialize VM memory to
//     disk. For large VMs (multi-GB RAM) this can take seconds. Incremental
//     or copy-on-write snapshot strategies (like CRIU) are not yet explored.
//
//   - Wake-from-snapshot relies on the snapshot being on local storage.
//     Cross-node wakes (migrating a hibernated sandbox to a different node)
//     require the snapshot to be in shared storage (Erebus/S3). This
//     migration path is not implemented.
//
//   - There is no pre-emptive hibernation policy. Hypnos only responds to
//     explicit control-plane commands. An idle-detection policy (e.g.,
//     "hibernate after N minutes of no requests") is tracked in Persephone
//     but not wired to Hypnos.
//
//   - The [Manager] does not emit Prometheus metrics for hibernate/wake
//     latency or failure rates.
package hypnos
