# TASKS

Phase 3 (v1.0) is complete. The items below capture the remaining work, organized by roadmap phase and ordered by immediacy.

## Phase 4 — The Titans Awaken (Immediate)
- [x] **Erebus v2.0 (OCI pipeline)**: Implement `OCIBuilder` to pull OCI images, extract layers, and assemble bootable rootfs disks; add layer caching/deduplication; support private registries (Docker Hub/GCR/ECR) and cache inspection; integrate with Nyx template creation and the CLI.
- [x] **Nyx data-science snapshots**: Ship warmed templates (`python-ds`, `pytorch-ml`, `r-analytics`, `julia-sci`) that preload heavy libs before snapshotting; automate warming after base image updates; enforce <200ms cold starts and publish benchmark results.
- [x] **Phlegethon resource classes**: Define resource classes (ember/flame/blaze/inferno) including GPU-aware `ClassInferno`; create dedicated node pools and scheduler hooks; expose metrics for routing decisions and hit rates.
- [x] **Typhon quarantine hardening**: Provide isolated storage backend, stricter seccomp profiles, and default-no-network posture for quarantine; add auto-classification hooks and manual override paths; create e2e tests proving isolation.
- [x] **Hypnos hibernation**: Finish `SleepManager` (pause/compress/write/restore); add lifecycle hooks in agent/runtime; expose API/CLI toggles; gate with `EnableHypnos` and promote to default once stability/latency targets are met.

## Phase 5 — Ascension to Olympus (Near-Term Production)
- [x] **Cerberus authentication/RBAC**: Build API key + OIDC flows, mTLS for agent links, role/tenant-aware authorization, and access auditing; wire into Judges and CLI.
- [x] **Charon traffic ferry**: Load balancing across Olympus instances with health checks, circuit breaking, retries, and per-tenant rate limiting.
- [x] **CLI v2.0**: Add `tartarus init template` (Dockerfile/OCI to template), `logs --follow`, `snapshot create/list/delete`, `exec` and `inspect`, config management, and shell completions.
- [x] **Kubernetes integration**: Deliver CRI shim or Operator/CRDs (`SandboxJob`, `SandboxTemplate`) to schedule pods into Tartarus microVMs; validate end-to-end lifecycle and network/policy parity.
- [ ] **Observability dashboard**: Grafana-ready dashboards for control-plane health, routing, enforcement, and template performance; live sandbox views and capacity heatmaps.
- [ ] **Security hardening**: Harden guest kernel options, generate seccomp profiles per class, automate template vulnerability scans, and integrate secrets delivery (Vault/KMS).

## Phase 6 — The Golden Age (Future)
- [ ] **Persephone predictive scaling**: Learn usage seasonality, prewarm/hibernate node pools, and apply scheduled scale rules with feedback from Hermes metrics.
- [ ] **Thanatos graceful termination**: Enable checkpoint/save-on-signal flows, grace-period enforcement, and automatic handoff to Hypnos where applicable; remove `EnableThanatos` guard after validation.
- [ ] **Kampe legacy compatibility**: Provide Docker/containerd/gVisor adapters and migration tooling to move container workloads into microVMs under the same scheduler.
- [ ] **Unified runtime + WASM**: Add WASM runtime option with automatic isolation selection and shared interface across microVM/WASM/gVisor.
- [ ] **Ecosystem & docs**: Publish full documentation site, tutorials, template marketplace, Terraform provider, GitHub Actions, and VS Code extension to support community adoption.
