// Package kampe implements the legacy runtime shim — the Monster Jailer who
// bridges standard container runtimes into the Tartarus microVM ecosystem.
//
// Named after the she-dragon jailer who guarded the Titans, Kampe provides
// migration and compatibility shims for Docker and containerd workloads,
// allowing existing containerised applications to be lifted into Tartarus
// without code changes.
//
// # Implementations
//
//   - [DockerRuntime]: Wraps the Docker daemon API to manage containers as
//     if they were Tartarus sandboxes. Provides a [tartarus.SandboxRuntime]-
//     compatible interface over the Docker socket.
//
//   - [ContainerdRuntime]: Similar to DockerRuntime but speaks directly to
//     containerd via its gRPC API. Lower overhead than Docker.
//
//   - [GVisorRuntime]: gVisor-specific shim for Kampe; distinct from the
//     primary [tartarus.GVisorRuntime] in that it is focused on compatibility
//     with OCI container specs from existing workloads.
//
//   - [ParityHarness]: Test harness that runs the same workload through both
//     the legacy and native runtimes and compares output for correctness
//     during migration validation.
//
// # Migration Path
//
// [Migration] orchestrates the automated promotion of a Docker image from a
// legacy runtime into a native Tartarus template, including layer extraction
// (Erebus), snapshot creation (Nyx), and template registration (Olympus).
//
// # Known Technical Debt
//
//   - [DockerRuntime] lacks resource limit enforcement; CPU/memory cgroups are
//     configured at container creation but not dynamically updated during
//     runtime. This diverges from the Firecracker and gVisor runtimes which
//     enforce limits via the VMM.
//
//   - [Migration] does not handle multi-stage Dockerfiles. Only the final
//     image layer is promoted; intermediate build artefacts are discarded.
//
//   - [ParityHarness] compares stdout output byte-for-byte. Non-deterministic
//     workloads (timestamps, random output) will always fail parity checks.
//     A semantic diff or structured output comparison is needed.
//
//   - [ContainerdRuntime] is not covered by any integration tests because
//     the CI environment does not have containerd running. The implementation
//     is best-effort and may have undiscovered regressions.
package kampe
