# Tartarus Project: Remaining Tasks & Technical Debt

This document outlines the remaining work for the Tartarus project, based on an analysis of `ROADMAP.md` and the current codebase state.

## � Critical Technical Debt

These items are marked as TODO in the codebase and need immediate attention.

- [x] **Erebus (Storage)**: Implement cleanup of files when snapshots are deleted.
  - Location: `pkg/nyx/local_manager.go` (`// TODO: Delete files from Erebus?`)
- [ ] **Olympus (Scaler)**: Add more metrics for scaling decisions (CPU/Memory utilization).
  - Location: `pkg/olympus/scaler.go` (`// TODO: Add more metrics like CPU/Mem util`)
- [ ] **Hecatoncheir (Agent)**: Runtime.StreamLogs support for follow flag.
  - Location: `pkg/hecatoncheir/agent.go`
- [ ] **Erebus (OCI)**: Dynamic init binary location.
  - Location: `pkg/erebus/oci.go`
- [ ] **Persephone**: Calculate actual forecast confidence.
  - Location: `pkg/persephone/forecast.go`
- [ ] **Tests**: Instrumentation for actual phase timing in benchmarks.
  - Location: `tests/perf/python_ds_bench_test.go`

## 🛠 Feature Verification (Phase 5: Ascension to Olympus)

Code exists for these components, but they need verification and integration testing to be considered "Done".

- [ ] **Cerberus (Auth Gateway)**: Verify API key/OAuth2 implementation and RBAC enforcement.
  - Pkg: `pkg/cerberus`
- [ ] **Charon (Load Balancer)**: Verify request routing, rate limiting, and circuit breaker logic.
  - Pkg: `pkg/charon`
- [ ] **Kubernetes Operator**: Verify CRD reconciliation (`SandboxJob`) and full lifecycle in K8s.
  - Pkg: `pkg/kubernetes`
- [ ] **Observability Dashboard**: Finalize Grafana templates and dashboards.
  - Current: `docker-compose.observability.yml` exists, verify dashboard JSONs in `config/grafana/dashboards`.

## 🛠 Feature Verification (Phase 6: The Golden Age)

Advanced features that have implementation but need hardening.

- [ ] **Unified Runtime**: Verify automatic selection logic (WASM vs MicroVM vs gVisor).
  - Pkg: `pkg/tartarus/unified_runtime.go`
  - [ ] Verify WASM Runtime (`pkg/tartarus/wasm_runtime.go`) execution.
- [ ] **Persephone (Seasonal Scaling)**: Verify predictive scaling and pre-warming logic.
  - Pkg: `pkg/persephone`
- [ ] **Thanatos (Graceful Termination)**: Verify checkpoint creation and graceful signal handling.
  - Pkg: `pkg/thanatos`

## 🔮 Future / Missing Features

Items from the Roadmap/Vision that appear unimplemented.

- [ ] **Seccomp Profile Generator**: Automated profile generation for guest kernels (Roadmap 5.5).
- [ ] **Tartarus CLI v2.0**: Missing commands.
  - `tartarus init template`
  - `tartarus snapshot` management commands
  - `tartarus exec` implementation
- [ ] **Security Hardening**:
  - Guest kernel hardening (grsecurity-inspired).
  - Secrets injection via Vault/KMS integration (check `pkg/cerberus` for this).

## 📦 Ecosystem

- [ ] **VS Code Extension**: Address TypeScript definition issues.
