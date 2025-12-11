# Tartarus Project: Remaining Tasks & Technical Debt

This document outlines the remaining work for the Tartarus project, based on an analysis of `ROADMAP.md` and the current codebase state.

## � Critical Technical Debt

These items are marked as TODO in the codebase and need immediate attention.

- [x] **Erebus (Storage)**: Implement cleanup of files when snapshots are deleted.
  - Location: `pkg/nyx/local_manager.go` (`// TODO: Delete files from Erebus?`)
- [x] **Olympus (Scaler)**: Add more metrics for scaling decisions (CPU/Memory utilization).
  - Location: `pkg/olympus/scaler.go` (implemented: CPU/Memory utilization, queue depth, launch/error counts)
- [x] **Hecatoncheir (Agent)**: Runtime.StreamLogs support for follow flag.
  - Location: `pkg/hecatoncheir/agent.go` (implemented: follow flag parsed from control message and passed to Runtime.StreamLogs)
- [x] **Erebus (OCI)**: Dynamic init binary location.
  - Location: `pkg/erebus/oci.go` (Implemented dynamic search with fallbacks)
- [x] **Persephone**: Calculate actual forecast confidence.
  - Location: `pkg/persephone/forecast.go` (Already implemented using MSE-based confidence calculation)
- [x] **Tests**: Instrumentation for actual phase timing in benchmarks.
  - Location: `tests/perf/python_ds_bench_test.go` (PhaseTimer instrumentation in TestPythonDSColdStartWithHarness)

## 🛠 Feature Verification (Phase 5: Ascension to Olympus)

Code exists for these components, but they need verification and integration testing to be considered "Done".

- [x] **Cerberus (Auth Gateway)**: Verify API key/OAuth2 implementation and RBAC enforcement.
  - Pkg: `pkg/cerberus` (26+ tests passing: API key, JWT, mTLS, OIDC, RBAC, middleware)
- [x] **Charon (Load Balancer)**: Verify request routing, rate limiting, and circuit breaker logic.
  - Pkg: `pkg/charon` (26 tests passing: 5 LB strategies, token bucket rate limiting, 3-state circuit breaker)
- [x] **Kubernetes Operator**: Verify CRD reconciliation (`SandboxJob`) and full lifecycle in K8s.
  - Pkg: `pkg/kubernetes` (5 tests passing: SandboxJob, SandboxTemplate, TenantNetworkPolicy controllers)
- [x] **Observability Dashboard**: Finalize Grafana templates and dashboards.
  - 4 dashboards verified: `control_plane.json`, `phase4-slos.json`, `resources.json`, `topology.json`

## 🛠 Feature Verification (Phase 6: The Golden Age)

Advanced features that have implementation but need hardening.

- [x] **Unified Runtime**: Verify automatic selection logic (WASM vs MicroVM vs gVisor).
  - Pkg: `pkg/tartarus/unified_runtime.go`
  - [x] Verify WASM Runtime (`pkg/tartarus/wasm_runtime.go`) execution.
- [x] **Persephone (Seasonal Scaling)**: Verify predictive scaling and pre-warming logic.
  - Pkg: `pkg/persephone`
- [x] **Thanatos (Graceful Termination)**: Verify checkpoint creation and graceful signal handling.
  - Pkg: `pkg/thanatos`

## 🔮 Future / Missing Features

Items from the Roadmap/Vision that appear unimplemented.

- [x] **Seccomp Profile Generator**: Automated profile generation for guest kernels (Roadmap 5.5).
  - [x] Implement `SeccompProfileGenerator` in `pkg/typhon` to support template-based generation.
  - [x] Implement `AnalyzeStrace` to learn syscalls from strace output.
  - [x] Add `tartarus seccomp generate` CLI command.
  - [x] Verify with unit tests and a manual run.
- [ ] **Tartarus CLI v2.0**: Missing commands.
  - `tartarus init template`
  - `tartarus snapshot` management commands
  - `tartarus exec` implementation
- [ ] **Security Hardening**:
  - Guest kernel hardening (grsecurity-inspired).
  - Secrets injection via Vault/KMS integration (check `pkg/cerberus` for this).

## 📦 Ecosystem

- [ ] **VS Code Extension**: Address TypeScript definition issues.
