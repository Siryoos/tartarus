// Package nyx implements the snapshot manager — the Primordial Night that
// preserves and restores VM memory state for rapid cold starts.
//
// Named after the primordial goddess of night who holds secrets in darkness,
// Nyx manages the creation, storage, and restoration of microVM snapshots.
// By pre-warming a VM to a ready state and taking a snapshot, subsequent
// launches restore the snapshot rather than cold-booting, reducing latency
// from seconds to milliseconds.
//
// # Core Types
//
//   - [Manager]: Interface for snapshot lifecycle operations (Create, Restore,
//     List, Delete).
//
//   - [LocalManager]: Implementation that stores snapshots on the local
//     filesystem. Suitable for single-node and development deployments.
//
//   - [LocalManagerStub]: A no-op stub used in test builds or environments
//     where the Firecracker SDK is not available.
//
// # Snapshot Lifecycle
//
//  1. A warm-up run executes the sandbox init sequence.
//  2. [Manager.Create] serialises VM memory and disk diff to the snapshot dir.
//  3. On subsequent launches, [Manager.Restore] loads the snapshot, skipping
//     the boot sequence entirely.
//  4. [Manager.Delete] removes the snapshot directory and frees disk space.
//
// # Known Technical Debt
//
//   - Snapshot files are stored on local disk only. There is a placeholder
//     comment in [LocalManager] about deleting files from Erebus (S3) when
//     a snapshot is deleted, but this cross-store cleanup is not implemented.
//     Orphaned S3 objects can accumulate.
//
//   - There is no snapshot versioning or differential (incremental) snapshot
//     support. Each [Manager.Create] writes a full memory dump. For large
//     VMs this is slow and disk-intensive.
//
//   - The warm-up benchmark ([warmup_test.go]) measures restore latency but
//     the results are not fed back into Persephone's capacity planning. The
//     integration between Nyx and Persephone is manual.
//
//   - There is no eviction policy for old snapshots. Disk space will grow
//     without bound unless an operator manually prunes old entries.
package nyx
