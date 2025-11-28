# Tartarus Phase 3 Closeout (v1.0) Tasks

Context: ROADMAP.md marks Phase 3 as in progress and the prior task board shows completed items, but code review surfaces several gaps. This file tracks what must land before the first stable release.

## Critical Technical Debt (Must Fix)
- [x] Acheron RedisQueue Ack scaling (`pkg/acheron/redis_queue.go`)
  - Current: `Ack` scans the entire processing list (O(N)) because the interface only passes an ID.
  - DoD: Refactor queue interface to return a receipt/handle or store processing items by ID so `Ack` is O(1); update `MemoryQueue` and `Agent` call sites; add tests proving bounded-time ack for large processing lists.
- [x] Acheron Nack crash-safety and corrupt payload handling (`pkg/acheron/redis_queue.go`)
  - Current: Nack depends on unmarshaling before MULTI/EXEC; corrupt JSON can loop or be dropped without audit.
  - DoD: Introduce dead-letter path for invalid payloads (Cocytus), ensure atomic move back to queue or DLQ, surface metrics for poison pills and nack failures, and add regression tests.

## Missing Phase 3 Features (per ROADMAP)
- [ ] Aeacus (Audit Judge) implementation (`pkg/judges`)
  - DoD: Add AeacusJudge to tag compliance/retention metadata and emit audit records; wire into `judges.Chain` in `cmd/olympus-api/main.go`; include unit tests and sample audit output.
- [ ] Advanced scheduling (affinity/anti-affinity and bin-packing) (`pkg/moirai`)
  - DoD: Extend scheduler to honor placement hints/labels and provide a bin-packing strategy in addition to least-loaded; expose config toggle; add tests covering anti-affinity and tight packing scenarios.
- [ ] Megaera runtime network watchdog (`pkg/erinyes`)
  - DoD: Monitor live network usage/egress against policy (bandwidth caps, banned IP attempts) during execution, not just Styx setup-time rules; enforce via Erinyes kill path with metrics and logs.

## Persistence and Durability Gaps
- [ ] Olympus control-plane persistence verification (`pkg/olympus`, `pkg/hades`)
  - Current: defaults to in-memory registry when Redis is unset; TASKS claimed persistence was done.
  - DoD: Ensure production config uses Redis-backed registry/queue by default, add restart recovery test (state survives manager restart), and document required settings.
- [ ] Themis policy durability and versioning (`pkg/themis`)
  - Current: `MemoryRepo` is volatile and has no versioning.
  - DoD: Provide Redis/SQL/file-backed repo with version stamps and optimistic updates; load policies on startup; add API/tests for list/get/upsert that survive restart.
- [x] Agent poison-pill handling (`pkg/hecatoncheir/agent.go`, `pkg/acheron/redis_queue.go`)
  - Current: Dequeue or JSON decode failures are logged but not sent to Cocytus; risk of retry loops or silent drops.
  - DoD: On decode failure, emit to Cocytus with payload snapshot, ack/drop from queue to prevent loops, and record metrics; add test covering corrupt message flow.

## Release Validation
- [ ] Regression suite for Phase 3 paths
  - Cover queue ack/nack behavior, Aeacus audit tagging, scheduler affinity/bin-pack decisions, Megaera network kills, and persistence across restarts.
- [ ] Documentation refresh
  - Update ROADMAP.md and user guides to mark Phase 3 completion, config defaults (Redis/Hades/Themis), and note Phase 4+ items (Typhon, Charon, Hypnos) as future work.
