// Package domain defines the canonical data model for the Tartarus platform.
//
// This package is the single source of truth for all shared types, IDs, and
// status constants. It has no external dependencies within this repository;
// all other packages import from domain, not the reverse.
//
// # Core Types
//
//   - [SandboxRequest]: The unit of work that flows through the system. It
//     encapsulates the template to run, resource requirements, environment,
//     secrets, and scheduling metadata. Produced by Olympus and consumed by
//     Hecatoncheir via Acheron.
//
//   - [SandboxRun]: The lifecycle record of a running or completed sandbox.
//     Written by Hecatoncheir and read by Olympus and Hades. Includes a
//     [MemorySource] annotation indicating how memory usage was tracked.
//
//   - [NodeStatus]: The snapshot of a worker node's capacity and health,
//     maintained by Hades and read by Moirai for scheduling decisions.
//
//   - [Profile]: A strongly-typed instance profile (e.g. "phlegethon.large")
//     with validation and component accessors.
//
//   - [Envelope]: A versioned wrapper for JSON serialised structs to support
//     schema evolution in Redis and over the wire.
//
// # ID Types
//
// Strongly typed ID aliases prevent accidental interchange of different
// identifier namespaces:
//   - [SandboxID]  — uniquely identifies a sandbox request and its run
//   - [TemplateID] — identifies an execution template
//   - [NodeID]     — identifies a worker node
//   - [SnapshotID] — identifies a VM/WASM snapshot
//   - [PolicyID]   — identifies a Themis policy
//
// # Isolation Types
//
// [IsolationType] selects the runtime backend:
//   - [IsolationMicroVM]  — Firecracker MicroVM
//   - [IsolationWASM]     — WebAssembly via wazero
//   - [IsolationGVisor]   — gVisor (runsc)
//   - [IsolationAuto]     — automatic selection by the Unified Runtime
//
// # Known Technical Debt
//
//   - The scope of `Profile` tier constants. While the `Profile` type is here,
//     specific tier constants (e.g. `ProfileEmber`) are currently defined in
//     `pkg/phlegethon` to mirror `HeatLevel`. As the system grows, we may need
//     to re-evaluate if these should move to `domain`.
package domain
