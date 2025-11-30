# Tartarus Phase 3 Closeout / v1.0 Release Tasks

Context: ROADMAP.md marks Phase 3 as in progress, and code review shows components built but wiring gaps. This checklist captures what is required to declare v1.0 ready.

## Control Plane Wiring Gaps
- [x] Phlegethon heat routing into Olympus (`pkg/olympus/manager.go`, `pkg/phlegethon`)
  - Current: Manager bypasses Phlegethon and calls `Scheduler.ChooseNode` directly.
  - DoD: Call `Phlegethon.ClassifyHeat(req)` before scheduling; pass heat level to Moirai; add tests showing heat-aware placement.
- [x] Quarantine enforcement (Typhon) (`pkg/olympus/manager.go`, `pkg/moirai`)
  - Current: VerdictQuarantine sets `req.Metadata["quarantine"]=true` but scheduler ignores it; Typhon manager is in-memory only.
  - DoD: Scheduler must route quarantine jobs only to nodes labeled for Typhon (or reject if none); add tests for quarantine anti-affinity and fallback behavior.
- [x] Aeacus audit judge wiring (`cmd/olympus-api/main.go`, `pkg/judges`)
  - Current: Audit judge presence is unverified in Olympus wiring; agent chain is empty (acceptable).
  - DoD: Ensure AeacusJudge is constructed and included in `judges.Chain` for Manager; emit audit records with sample sink; add unit/integration tests.

## Critical Technical Debt
- [x] Acheron RedisQueue Ack scalability (`pkg/acheron/redis_queue.go`, `pkg/acheron/queue.go`)
  - Current: Ack scans entire processing list (O(N)).
  - DoD: Refactor interface to pass a receipt/full item or change processing storage for O(1) Ack; update MemoryQueue and agent call sites; add performance/regression tests.
- [x] Acheron Nack crash-safety and DLQ for corrupt payloads (`pkg/acheron/redis_queue.go`)
  - Current: Nack unmarshals before MULTI/EXEC; corrupt JSON can loop or be dropped.
  - DoD: Add dead-letter path for invalid payloads, ensure atomic move back to queue or DLQ, and emit metrics/tests for poison pills.
- [x] Agent poison-pill handling (`pkg/hecatoncheir/agent.go`, `pkg/acheron/redis_queue.go`)
  - DoD: On dequeue/unmarshal failure, write to Cocytus with payload snapshot, ack/drop to avoid loops, and expose metrics; add tests.

## Persistence and Durability
- [x] Olympus/Hades persistence verification (`pkg/olympus`, `pkg/hades`)
  - Current: TASKS claimed done, but default wiring uses in-memory when Redis unset.
  - DoD: Default production config to Redis-backed registry/queue, verify state survives manager restart, and document required settings.
- [x] Themis policy durability and versioning (`pkg/themis`)
  - Current: MemoryRepo only.
  - DoD: Provide Redis/SQL/file-backed repo with version stamps and optimistic updates; load policies on startup; add persistence tests.

## Stability Guardrails
- [x] Hypnos/Thanatos gating (`pkg/hecatoncheir/agent.go`, config)
  - Current: Phase 4 components imported/initialized in agent.
  - DoD: Default to noop/disabled in v1.0 configs, guard code paths with feature flags, and add tests to ensure no accidental hibernation/termination.

## Release Validation
- [x] Regression suite for Phase 3 behaviors
  - Created comprehensive test suite:
    - `pkg/integration/phase3_heat_regression_test.go` - Heat-aware scheduling (4 test cases)
    - `pkg/integration/phase3_quarantine_regression_test.go` - Quarantine placement (5 test cases)
    - `pkg/integration/phase3_dlq_regression_test.go` - DLQ and poison pill handling (5 test cases)
    - Enhanced `pkg/integration/phase3_regression_test.go` - Main integration test with documentation
  - All tests passing, covers: heat-aware scheduling, quarantine placement, ack/nack/DLQ flows, Aeacus audit logging, persistence across restarts
  - Hypnos/Thanatos gating covered by existing `pkg/hecatoncheir/agent_guardrails_test.go`
- [x] Documentation refresh
  - Updated ROADMAP.md with Phase 3 completion summary, component status updates (Phlegethon, Typhon), and Phase 4 gating notes
  - Enhanced docs/persistence.md with Phlegethon heat routing and Typhon quarantine configuration
  - Updated docs/DEV_STACK.md with Phase 3 component details and feature flag documentation
  - Created docs/configuration.md with comprehensive production configuration reference
