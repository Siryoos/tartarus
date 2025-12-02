# Technical Debt and Remaining Tasks

## Phase 4 Debt — Verification of the Titans
- [x] Bench `python-ds` cold starts to prove <200ms target; add repeatable perf harness under `tests/perf` with Hermes metrics and regression alerts.
- [x] Time OCI image -> rootfs conversion (<30s) in `pkg/erebus` using representative large images; profile extraction/dedupe/init injection and document cache hit behavior.
- [x] Measure Typhon quarantine routing overhead (<50ms) with normal vs quarantine comparison; add integration test that exercises seccomp isolation and reports added latency.
- [x] Validate Hypnos wake-from-sleep (<100ms); enable feature flags in staging, capture hibernation/resume traces, and gate releases on SLO.
- [x] Publish a Phase 4 performance report and dashboards (Grafana/Prometheus) covering the above SLOs to prevent regression.

## Phase 5 — Ascension to Olympus (Production Hardening)
### Cerberus (Auth Gateway)
- [x] Implement the three heads: API keys (signed/rotatable), OAuth2/OIDC (code + client creds), and mutual TLS for agents with automated cert rotation.
- [ ] Enforce RBAC/tenant scoping in `pkg/cerberus` gateway/middleware and thread identity through `cmd/olympus-api` handlers.
- [ ] Centralize audit logging and anomaly hooks to `pkg/hermes` with tamper-evident storage.
- [ ] Secrets retrieval via Vault/KMS providers for tokens, client secrets, and signing keys.

### Charon (Request Ferry)
- [ ] Production load balancer for Olympus instances with health checks, weighted/least-conn strategies, and consistent hashing for sticky sessions.
- [ ] Circuit breaker + retry + backoff middleware; tenant-aware rate limiting; end-to-end failover tests in `tests/integration/charon`.
- [ ] Telemetry for ferry decisions (success/fail/latency) exported to Hermes/Grafana.

### Observability and Security
- [ ] Grafana dashboards: real-time sandbox topology, resource usage heatmaps, error/latency SLOs; bundle with `docker-compose.observability.yml`.
- [ ] Security hardening: hardened guest kernel option, seccomp profile generator with per-template defaults, automated template vulnerability scanning, and Vault/KMS-backed secret injection flow.

### CLI v2.0 (`cmd/tartarus`)
- [ ] Implement interactive commands: `logs --follow` streaming from Hermes, `exec` attaching to running sandboxes, `inspect` detail view, snapshot create/list/delete, `init template`.
- [ ] Shell completions (bash/zsh) and config profiles; integration tests for streaming commands.

### Kubernetes Integration (`pkg/kubernetes`, `tartarus-operator`)
- [ ] CRI shim or SandboxJob operator wiring to Moirai/Hecatoncheir; CRDs for SandboxJob/SandboxTemplate with status conditions.
- [ ] End-to-end K8s conformance (overhead <1s), Helm chart, and multi-tenant network/policy mapping.

## Phase 6 — The Golden Age (Future Capabilities)
### Persephone (Seasonal Scaling)
- [ ] Predictive autoscaler that learns historical demand, trains forecasts, and pre-warms node pools; data pipeline plus evaluation harness.
- [ ] Seasonal schedules with budget caps and Hypnos integration for hibernation cycles.

### Thanatos (Graceful Termination)
- [ ] Graceful shutdown controller with checkpoint/export before kill; policy-driven grace windows; integration with Erinyes enforcement and Hermes audit.
- [ ] User/API surface for deferred termination and resume-from-checkpoint flows.

### Kampe (Legacy Runtime)
- [ ] Docker/containerd/gVisor adapters completing `pkg/kampe`; parity tests ensuring OCI workload behavior matches microVM runtime.
- [ ] Migration tooling to move running containers to Firecracker VMs with state export/import.

### Unified Runtime (`pkg/tartarus`)
- [ ] WASM runtime integration (WasmEdge/Wasmtime) behind `IsolationAuto` selector; routing logic choosing WASM vs microVM based on workload density/cost.
- [ ] Performance matrix comparing WASM vs microVM vs gVisor; SLO alerts for regression.

## Ecosystem and Integration Plane
- [ ] GitHub Actions integration (sandboxed runners) and VS Code extension for template/run/debug flows.
- [ ] Template marketplace and docs site refresh with tutorials; plugin system for custom judges/furies.
