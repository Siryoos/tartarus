// Package tartarus implements the microVM runtime layer — the Abyss itself,
// where sandboxes are born and where they end their days.
//
// Named after the deep abyss below Hades where the Titans were imprisoned,
// this package provides the [SandboxRuntime] interface and its concrete
// implementations: Firecracker, gVisor, and WebAssembly. It also provides
// the [UnifiedRuntime] which automatically selects the best backend.
//
// # Core Types
//
//   - [SandboxRuntime]: The primary interface. All callers (Hecatoncheir,
//     Hypnos, Erinyes) depend on this interface, not on concrete types.
//
//   - [FirecrackerRuntime]: Production-grade microVM runtime using the
//     Firecracker SDK. Provides strong isolation with ~125 ms cold-start
//     and sub-millisecond snapshot restore. Linux-only (build tag: linux).
//
//   - [GVisorRuntime]: Container runtime using gVisor's runsc. Provides
//     strong syscall-level isolation with good Docker compatibility.
//     Supports both ptrace and KVM platforms.
//
//   - [WasmRuntime]: Lightweight runtime using wazero for WASM workloads.
//     Provides near-zero cold-start with process-level isolation.
//
//   - [UnifiedRuntime]: Wraps all three runtimes and selects automatically
//     based on the request's [domain.IsolationType] or workload heuristics.
//
// # Runtime Selection (UnifiedRuntime)
//
//   - Explicit: if req.Metadata["isolation"] is set, that runtime is used.
//   - Auto: WASM modules (*.wasm) → WasmRuntime; hardened=true → GVisor;
//     default → FirecrackerRuntime.
//
// # Known Technical Debt
//
//   - [FirecrackerRuntime.Exec] and [FirecrackerRuntime.ExecInteractive]
//     return ErrNotImplemented. Exec inside a Firecracker VM requires a
//     guest agent communicating over a vsock socket. This guest agent is
//     not yet implemented.
//
//   - [GVisorRuntime.Status] attempts to read memory usage from the runsc
//     state but this is currently not implemented (the comment reads
//     "not implemented here"). [domain.SandboxRun.MemoryUsage] will be
//     zero for gVisor sandboxes.
//
//   - [FirecrackerRuntime] requires the Firecracker binary, a kernel image,
//     and a root filesystem to be pre-provisioned on the host. There is no
//     automated provisioning path; operators must prepare these manually.
//
//   - [WasmRuntime] does not support WASI network sockets. WASM workloads
//     that require network access (common for web servers or data pipelines)
//     cannot use this runtime.
//
//   - The [MockRuntime] in mock_runtime.go is exported and may be accidentally
//     imported by non-test code. It should be moved to a test-only file
//     (mock_runtime_test.go or a testutil sub-package).
package tartarus
