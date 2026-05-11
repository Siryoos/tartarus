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
//     Written by Hecatoncheir and read by Olympus and Hades.
//
//   - [NodeStatus]: The snapshot of a worker node's capacity and health,
//     maintained by Hades and read by Moirai for scheduling decisions.
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
//   - [ResourceSpec.Profile] (Phlegethon profile string) is parsed at the
//     call site in multiple packages. A dedicated Profile type with
//     parsing/validation logic should live here.
//
//   - [SandboxRun.MemoryUsage] is a best-effort field: only gVisor
//     populates it. Firecracker and WASM runtimes leave it zero.
//
//   - There is no versioning or schema evolution strategy for these types.
//     As the system grows, adding fields to JSON-serialised structs stored
//     in Redis (via Hades) must be done carefully to avoid decode failures
//     in mixed-version rollouts.
package domain
