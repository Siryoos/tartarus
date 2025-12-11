# Tartarus Project: Remaining Tasks & Technical Debt

This document outlines the remaining work for the Tartarus project, based on an analysis of `ROADMAP.md` and the current codebase state.

## 🚨 High Priority / Work In Progress

These items appear to be partially implemented or are immediate next steps.

- [x] **Complete gVisor Runtime Integration** (`pkg/tartarus/gvisor_runtime.go`)
  - [x] Implement `Launch` (convert `SandboxRequest` to OCI spec, create/start container)
  - [x] Implement `Kill`, `Pause`, `Resume` (call `runsc`)
  - [x] Implement `CreateSnapshot` (call `runsc checkpoint`)
  - [x] Implement `Wait` (poll state)
  - [x] Implement `Exec` (call `runsc exec`)
  - [x] Implement `StreamLogs`
- [x] **Enable and Verify Advanced Features**
  - [x] Enable Hypnos (Sleep/Hibernation) by default (currently gated by `EnableHypnos` flag)
  - [x] Enable Thanatos (Graceful Termination) by default (currently gated by `EnableThanatos` flag)
  - [x] Verify full integration of `pkg/hypnos` and `pkg/thanatos` with the main control plane.
- [x] **Kampe Legacy Runtime Shim**
  - [x] Verify status of `pkg/kampe` (currently listed as "Planned" in Roadmap, needs verification of completeness).

## 🛠 Technical Debt & Code Cleanup

Specific `TODO` and `FIXME` items found in the codebase.

### Critical
- [x] **Erebus (Storage)**: Implement cleanup of files.
  - `pkg/nyx/local_manager.go:367`: `// TODO: Delete files from Erebus?`
- [ ] **Persephone (Autoscaling)**: Improve forecast confidence calculation.
  - `pkg/persephone/forecast.go:132`: `// TODO: Calculate actual confidence`
- [ ] **Erebus (OCI)**: Dynamic init binary location.
  - `pkg/erebus/oci.go:229`: `// TODO: Locate the actual init binary`
- [ ] **Hecatoncheir (Agent)**: Log streaming improvements.
  - `pkg/hecatoncheir/agent.go:529`: `// TODO: Runtime.StreamLogs needs to support follow flag or we handle it here?`

### Performance & Testing
- [ ] **Olympus (Scaler)**: Add more metrics for scaling decisions.
  - `pkg/olympus/scaler.go:86`: `// TODO: Add more metrics like CPU/Mem util`
- [ ] **Tests**: Instrumentation for actual phase timing.
  - `tests/perf/python_ds_bench_test.go:589`: `// TODO: Implement actual phase timing instrumentation`

## 🔮 Future Phases (Roadmap)

### Phase 5: Ascension to Olympus (Multi-Region & Federation)
- [ ] **Unified Runtime Interface**: Abstract over Firecracker, gVisor, and Containers.
- [ ] **Charon (Load Balancer)**: Global request routing and load balancing.
- [ ] **Cerberus (Auth Gateway)**: Unified authentication and authorization.

### Phase 6: The Golden Age (Ecosystem)
- [ ] **Marketplace**: Template registry and sharing platform.
- [ ] **Federation**: Connecting multiple Tartarus clusters.
- [ ] **Advanced Billing**: Cost analysis and granular billing.

## 📦 Ecosystem
- [ ] **VS Code Extension**: Address TypeScript definition issues in `node_modules` (low priority, likely dependency related).
