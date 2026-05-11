# Tartarus Project: Remaining Tasks & Technical Debt

This document outlines the remaining work for the Tartarus project, based on an analysis of `ROADMAP.md` and the current codebase state.

## 🚨 Critical Technical Debt

These items are marked as TODO/placeholder in the codebase and need immediate attention.

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

---

## ⚠️ Remaining Technical Debt (by Package)

Catalogued from Go doc review and source-code audit (May 2026).

### `pkg/acheron` — Job Queue

- [x] **MemoryQueue context cancellation**: Replaced `sync.Cond.Wait` with a `chan struct{}` notify pattern. `Dequeue` now selects on `notify` and `ctx.Done()` simultaneously, returning `ctx.Err()` immediately on cancellation.
- [x] **MemoryQueue has no telemetry**: Extracted `metricsQueue` decorator (`NewInstrumentedQueue`) that wraps any `Queue` and emits the same counter/gauge set as `RedisQueue`. `NewMemoryQueue` now accepts `hermes.Metrics` and is instrumented by default.
- [x] **No priority queuing**: Added `domain.Priority` type (Low/Normal/High) and `PriorityQueue` — a multi-tier fan-in with one `MemoryQueue` per level. `Dequeue` always drains the highest non-empty tier first while respecting context cancellation.
- [x] **No Redis Cluster support**: Added `RedisClusterQueue` backed by `redis.ClusterClient`. Keys are hash-tagged (`{streamKey}` / `{streamKey}:dlq`) so both land on the same shard, satisfying the Lua script cross-key constraint.

### `pkg/cerberus` — Auth Gateway

- [ ] **No doc.go technical debt section** — existing docs are comprehensive. No additional items identified.

### `pkg/charon` — Load Balancer / Ferry

- [x] **Consistent-hash sticky sessions**: Added `StickySessionTable` (`sticky_session.go`) that pins `sessionKey → shoreID` on first hash selection. `DeregisterShore` calls `Drain(shoreID)` to mark entries as draining (not deleted); a background sweeper evicts them after `StickySessionDrainTimeout` (default 5 min). `selectConsistentHash` checks the table first and re-pins on re-selection.
- [x] **In-memory rate limiting only**: Added `RedisRateLimiter` (`rate_limiter_redis.go`) using a sliding-window Lua script on `redis/go-redis/v9`. Auto-selected in `NewBoatFerry` when `RateLimitConfig.RedisAddr` is non-empty; falls back to `TokenBucketLimiter` otherwise.
- [x] **No mTLS health check probes**: `HealthCheck` gains `TLSConfig *tls.Config`; `AddShore` builds a per-shore `http.Client` with a custom TLS transport when set. `NewMTLSHealthCheck(certFile, keyFile, caFile, path)` provides ergonomic construction.
- [x] **No WebSocket/streaming proxy**: Added `websocket_proxy.go` with bidirectional gorilla WS tunnel (`proxyWebSocket`) and SSE/long-poll streaming (`proxyStream`). `FerryMiddleware` stores the `ResponseWriter` in context; `forwardRequest` detects `Upgrade: websocket` / `Accept: text/event-stream` and routes to the appropriate helper.
- [x] **Zone-aware routing is parsed but not used**: Implemented `selectZoneAware` in `load_balancer.go`. Prefers shores matching `X-Zone` header or `FerryConfig.LocalZone`; falls back to global round-robin. `StrategyZoneAware` case wired into `selectShore`.

### `pkg/cocytus` — Error Stream

- [ ] **No durable sink**: Only a `LogSink` is provided. Production deployments need a sink that persists to Acheron DLQ or sends alerts (Slack/PagerDuty).
- [ ] **No fan-out (multi-sink)**: The interface accepts a single Sink. A `MultiSink` wrapper is needed.
- [ ] **No replay capability**: Failed records are logged only; no retry or replay mechanism exists.

### `pkg/domain` — Core Types

- [ ] **No `Profile` type**: `ResourceSpec.Profile` is parsed by multiple packages via string conventions. A dedicated `Profile` type with validation should live in `domain`.
- [ ] **`SandboxRun.MemoryUsage` is best-effort**: Only gVisor populates it; Firecracker and WASM leave it zero.
- [ ] **No schema evolution strategy**: JSON-serialised structs in Redis (via Hades) have no versioning. Mixed-version rollouts risk decode failures.

### `pkg/erebus` — OCI / Artifact Store

- [ ] **Scanner integration is a stub**: Shells out to `trivy` if available, no enforcement policy, no fallback.
- [ ] **S3Store lacks multipart upload**: Single-object limit is ~5 GB. Large rootfs snapshots may fail silently.
- [ ] **No layer garbage collection**: Orphaned extracted layers accumulate on disk without an external pruning job.
- [ ] **No multi-arch manifest support**: OCI image index (multi-arch) manifests are not handled; always fetches first manifest.

### `pkg/erinyes` — Policy Enforcement / Furies

- [ ] **netlink namespace issue**: `NetworkStats` reads from host netlink socket. Inside Docker/K8s, per-sandbox accounting is incorrect.
- [ ] **CPU accounting is approximate**: Firecracker reports host-side jailer CPU, not guest vCPU time.
- [ ] **No violation debounce**: A sandbox oscillating around a threshold generates a storm of events. A dampening mechanism is needed.
- [ ] **Circular concern with runtime**: `erinyes` depends on `tartarus.SandboxRuntime` for polling, creating a tight coupling. A read-only metrics interface would decouple them.

### `pkg/hades` — Node Registry

- [ ] **O(N) LIST for large clusters**: `RedisRegistry` uses SCAN, introducing latency spikes at thousands of nodes. Needs indexed data structures.
- [ ] **No leader-election/quorum**: Split-brain scenarios can cause divergent scheduling decisions.
- [ ] **`ListRuns` is O(nodes × sandboxes)**: Should be replaced with a dedicated run index (Redis Set).
- [ ] **No label validation at registration**: Malformed node labels can silently break affinity scheduling.

### `pkg/hecatoncheir` — Node Agent

- [ ] **Reconcile doesn't handle host reboot**: Orphaned run records in Hades after a host reboot are never cleaned up.
- [ ] **Exec not implemented for Firecracker**: `FirecrackerRuntime.Exec` returns `ErrNotImplemented`. Requires vsock-based guest agent.
  - File: `pkg/tartarus/firecracker_runtime_exec.go`
- [ ] **Log streaming has no size limit**: Log files for long-running sandboxes grow without bound.
- [ ] **Single-goroutine worker loop**: Sandboxes are dequeued and launched serially. A worker-pool model with configurable concurrency is needed.
- [ ] **Snapshot has no completion ACK**: `ControlMessageSnapshot` has no reply channel; caller must poll.

### `pkg/hermes` — Telemetry / Observability

- [ ] **Distributed tracing is a stub**: `observability.go` exists but no spans are propagated across RPC or Pub/Sub calls. OpenTelemetry integration is incomplete.
- [ ] **No Gauge support in `Metrics` interface**: Requires direct use of Prometheus SDK, bypassing the façade.
- [ ] **Audit `Store.Read` is unimplemented**: The `Read` method is commented out as a placeholder. Audit retrieval/search is completely missing.
- [ ] **Anomaly detectors run synchronously**: Slow detectors add latency for all callers. An async queue is recommended.

### `pkg/hypnos` — Hibernation

- [ ] **No incremental snapshot support**: Full memory dumps for large VMs are slow and disk-intensive. CRIU-style incremental snapshots are not explored.
- [ ] **No cross-node wake/migration**: Wake-from-snapshot requires snapshot on local storage. Cross-node migration is not implemented.
- [ ] **No pre-emptive hibernation**: Hypnos only responds to explicit commands. Idle-detection policy is not wired from Persephone.
- [ ] **No metrics**: Hibernate/wake latency and failure rates are not emitted to Prometheus.

### `pkg/judges` — Admission Control

- [ ] **Integration test requires live Redis**: `Aeacus` integration test cannot run without Redis. Needs a mock for pure admission-logic testing.
- [ ] **No global quota enforcement**: `BasicJudge` checks per-request limits only. Tenant-wide concurrent sandbox counting (Redis INCR) is not implemented.
- [ ] **Rejections not in Prometheus**: Rejected requests are logged but not surfaced as metrics.
- [ ] **No CEL-based rule evaluation**: Dynamic admission expressions are roadmapped but not implemented.

### `pkg/kampe` — Legacy Runtime Shim

- [ ] **No dynamic resource limit enforcement**: `DockerRuntime` sets cgroups at creation time only.
- [ ] **No multi-stage Dockerfile support**: `Migration` only promotes the final image layer.
- [ ] **Parity harness is byte-for-byte**: Non-deterministic workloads always fail parity. Needs semantic diff.
- [ ] **`ContainerdRuntime` has no integration tests**: CI does not have containerd; implementation is unverified.

### `pkg/kubernetes` — Kubernetes Operator / CRDs

- [ ] **All CRDs are v1alpha1**: No conversion webhook; upgrading to v1beta1 requires full migration.
- [ ] **Incomplete status sub-resource updates**: Failed sandboxes may remain in "Pending" until next reconcile.
- [ ] **No Styx unavailability handling**: `TenantNetworkPolicyController` doesn't handle Styx being down at reconcile time.
- [ ] **No finaliser on SandboxJob**: Deleting a SandboxJob while the sandbox runs leaves an orphan.
- [ ] **No CRD validation webhooks**: Invalid specs are only rejected at controller reconcile, not at admission.

### `pkg/lethe` — Ephemeral Filesystem

- [ ] **CAP_SYS_ADMIN requirement**: overlayfs requires elevated privileges; some managed K8s nodes will use slow copy fallback.
- [ ] **No pool pre-warming**: Overlays are created on-demand. Persephone predictions are not wired into Lethe.
- [ ] **No queuing on pool exhaustion**: Callers are rejected immediately under heavy concurrency.
- [ ] **No orphan cleanup on startup**: Crashed overlays leave stale mount points.

### `pkg/moirai` — Scheduler

- [ ] **O(n) node scan**: Both strategies do linear scans. A sorted heap would give O(log n) complexity.
- [ ] **No network-topology awareness**: Rack/zone locality is ignored during placement.
- [ ] **No speculative reservation**: A node can fill up between scheduling and launch, causing wasted retries.
- [ ] **Affinity syntax is stringly-typed**: Prefix convention in metadata is fragile. A typed `AffinityRule` struct with CEL support would be safer.

### `pkg/nyx` — Snapshot Manager

- [ ] **No cross-store S3 cleanup**: Snapshots stored in Erebus/S3 are not deleted when `LocalManager.Delete` is called. Orphaned objects accumulate in S3.
- [ ] **No incremental/differential snapshots**: Every `Create` writes a full memory dump.
- [ ] **Warm-up results not fed to Persephone**: Integration between Nyx restore-latency measurements and Persephone's capacity planning is manual.
- [ ] **No snapshot eviction policy**: Disk space grows without bound.

### `pkg/olympus` — Control Plane

- [ ] **Basic net/http routing with no versioning**: Needs a proper router (chi/gorilla) and OpenAPI schema generation as the API grows.
- [ ] **`ListRuns` is O(nodes × runs)**: Too slow for large clusters on every API call. Needs a Redis run index.
- [ ] **In-memory template storage**: `MemoryTemplateManager` loses templates on restart. Needs Redis/etcd backend.
- [ ] **Synthetic pre-warm runs inflate metrics**: `Scaler` pre-warm calls are indistinguishable from real runs in `sandbox_submissions_total`.
- [ ] **No control command delivery ACK**: `RedisControlPlane` has no acknowledgement mechanism; commands may be silently dropped.

### `pkg/persephone` — Seasonal Scaling

- [ ] **Hand-rolled cron parser**: Supports limited subset of cron syntax; should use `github.com/robfig/cron`.
- [ ] **Prometheus query warnings silently discarded**: Partial data warnings during federation could corrupt pattern detection.
- [ ] **Linear demand model only**: Does not model non-linear spikes or decay. ARIMA/Prophet is planned but unimplemented.
- [ ] **Seasons stored in-memory**: Defined seasons are lost on Olympus restart.
- [ ] **Hibernation policy not wired to Hypnos**: `hibernation.go` logs hibernate commands but doesn't send them to the agent.

### `pkg/phlegethon` — Hot-Path Router

- [ ] **Static heat classification only**: Does not use dynamic signals (recent launch frequency, queue depth).
- [ ] **No explicit pre-warm API**: "Warm" tier uses snapshots if they exist but doesn't trigger creation of missing snapshots.
- [ ] **Manual node labelling**: No auto-labelling based on hardware capabilities (GPU, NVMe).
- [ ] **Not integrated with Charon**: Phlegethon and Charon make placement decisions independently, potentially conflicting.

### `pkg/plugins` — Extensibility Framework

- [ ] **Linux/macOS-only**: Go `plugin` package requires CGO; Windows and static builds are unsupported.
- [ ] **No plugin sandboxing**: A buggy plugin crashes the host process. Needs subprocess isolation (e.g., Hashicorp go-plugin).
- [ ] **No API version enforcement**: Plugin compiled against old API silently fails at runtime.
- [ ] **No hot-reload**: All plugin changes require a full process restart.

### `pkg/styx` — Network Gateway

- [ ] **Shell-exec iptables**: Fragile, slow, incompatible with nftables. Needs direct netlink or nftables API.
- [ ] **No CNI plugin integration**: Incompatible with Calico, Cilium, Flannel. Manual network namespace management.
- [ ] **Bandwidth limits not enforced**: tc (traffic control) integration is needed for egress shaping.
- [ ] **`HostGatewayStub` silently accepts all policies**: Network policy tests on non-Linux CI are effectively no-ops.

### `pkg/tartarus` — Runtime Layer

- [ ] **Firecracker Exec not implemented**: `FirecrackerRuntime.Exec` and `ExecInteractive` return `ErrNotImplemented`. Requires vsock guest agent.
  - File: `pkg/tartarus/firecracker_runtime_exec.go`
- [ ] **GVisor memory usage not populated**: `Status` cannot read gVisor memory; `SandboxRun.MemoryUsage` is always zero for gVisor.
  - File: `pkg/tartarus/gvisor_runtime.go` (line 267)
- [ ] **No automated Firecracker provisioning**: Binary, kernel image, and rootfs must be pre-provisioned manually.
- [ ] **WASM has no network support**: WASI network sockets are not supported; web server WASM workloads cannot use this runtime.
- [ ] **`MockRuntime` exported in non-test file**: `mock_runtime.go` should be in a `testutil` sub-package to prevent accidental import.

### `pkg/thanatos` — Graceful Termination

- [ ] **Checkpoint-on-exit silently skipped**: If Hypnos is unavailable, checkpoint is skipped without error propagation.
- [ ] **O(n) TTL scan per tick**: `Scheduler` uses `time.Ticker` with linear scan. For thousands of sandboxes, a min-heap is needed.
- [ ] **No Prometheus metrics for exit events**: Exit codes and termination reasons are not tracked.
- [ ] **Hardcoded signal sequence**: SIGTERM → SIGKILL. Some workloads need configurable signals (e.g., SIGHUP).

### `pkg/themis` — Policy Engine

- [ ] **No policy inheritance**: Flat per-template model; tenant-wide defaults require duplicated entries.
- [ ] **`MemoryRepository` concurrent safety**: `Set`/`Delete` operations need review for races with concurrent evaluations.
- [ ] **No CEL-based dynamic expressions**: Policy evaluation is simple field comparison only.
- [ ] **No audit trail for policy changes**: Policy modifications in Redis are not recorded via hermes/audit.

### `pkg/typhon` — Security / Quarantine

- [ ] **Network triggers never fire**: `network_egress`/`network_ingress` in `RuleBasedClassifier` are always zero; Erinyes metrics are not wired in.
  - File: `pkg/typhon/classifier.go` (line 83)
- [ ] **No `go:embed` for seccomp templates**: Template files must be shipped separately; binary is not self-contained.
- [ ] **Hardening not enforced post-launch**: `HardenedManager` applies hardening at launch only; no runtime enforcement.
- [ ] **CEL expressions compiled on every call**: `AutoQuarantineTrigger` conditions should be compiled once at load time and cached.

### CLI (`cmd/tartarus`)

- [ ] **Remote template download not implemented**: `template_marketplace.go:254` returns an error for templates not found locally; remote download is a stub.
  - File: `cmd/tartarus/cmd/template_marketplace.go` (line 254)

---

## 🛠 Feature Verification (Phase 5: Ascension to Olympus)

- [x] **Cerberus (Auth Gateway)**: Verify API key/OAuth2 implementation and RBAC enforcement.
  - Pkg: `pkg/cerberus` (26+ tests passing: API key, JWT, mTLS, OIDC, RBAC, middleware)
- [x] **Charon (Load Balancer)**: Verify request routing, rate limiting, and circuit breaker logic.
  - Pkg: `pkg/charon` (26 tests passing: 5 LB strategies, token bucket rate limiting, 3-state circuit breaker)
- [x] **Kubernetes Operator**: Verify CRD reconciliation (`SandboxJob`) and full lifecycle in K8s.
  - Pkg: `pkg/kubernetes` (5 tests passing: SandboxJob, SandboxTemplate, TenantNetworkPolicy controllers)
- [x] **Observability Dashboard**: Finalize Grafana templates and dashboards.
  - 4 dashboards verified: `control_plane.json`, `phase4-slos.json`, `resources.json`, `topology.json`

## 🛠 Feature Verification (Phase 6: The Golden Age)

- [x] **Unified Runtime**: Verify automatic selection logic (WASM vs MicroVM vs gVisor).
  - Pkg: `pkg/tartarus/unified_runtime.go`
  - [x] Verify WASM Runtime (`pkg/tartarus/wasm_runtime.go`) execution.
- [x] **Persephone (Seasonal Scaling)**: Verify predictive scaling and pre-warming logic.
  - Pkg: `pkg/persephone`
- [x] **Thanatos (Graceful Termination)**: Verify checkpoint creation and graceful signal handling.
  - Pkg: `pkg/thanatos`

## 🔮 Future / Missing Features

- [x] **Seccomp Profile Generator**: Automated profile generation for guest kernels (Roadmap 5.5).
  - [x] Implement `SeccompProfileGenerator` in `pkg/typhon` to support template-based generation.
  - [x] Implement `AnalyzeStrace` to learn syscalls from strace output.
  - [x] Add `tartarus seccomp generate` CLI command.
  - [x] Verify with unit tests and a manual run.
- [x] **Tartarus CLI v2.0**: All commands implemented.
  - [x] `tartarus init template` - Scaffold templates from Dockerfile or OCI images (`init.go`)
  - [x] `tartarus snapshot create/list/delete` - Snapshot management (`snapshot.go`)
  - [x] `tartarus exec` - Execute commands with interactive mode (`exec.go`)
  - [x] `tartarus logs --follow` - Stream logs with follow flag (`logs.go`)
  - [x] `tartarus inspect` - Detailed sandbox info (`inspect.go`)
  - [x] `tartarus config` - Configuration management (`config.go`)
  - [x] `tartarus completion` - Tab completion for bash/zsh/fish/powershell (`completion.go`)
  - [x] `tartarus resume` - Resume from checkpoint (`resume.go`)
  - [x] `tartarus checkpoints` - List checkpoints (`checkpoints.go`)
  - [ ] `tartarus init template <name>` - **Remote template download not implemented** (local-only)
- [x] **Security Hardening**:
  - [x] Guest kernel hardening (grsecurity-inspired).
  - [x] Secrets injection via Vault/KMS integration (check `pkg/cerberus` for this).

## 📦 Ecosystem

- [ ] **VS Code Extension**: Address TypeScript definition issues.
