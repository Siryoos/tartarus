# Task List (Architecture + First Release)

## Core platform gaps (target: first release)
- [x] Use Redis-backed Hades registry in the Hecatoncheir agent and send real heartbeats with allocations so Olympus can see nodes across processes.
- [x] Drive scheduling from Olympus: list nodes, pick via `pkg/moirai`, persist placement/run IDs, and route enqueue/control messages to the chosen agent (per-node queues or routing metadata).
- [x] Provide template + policy management so Nyx can prepare snapshots (kernel/rootfs paths) and Themis stores policies; expose basic APIs/fixtures for default templates.
- [x] Repair snapshot/overlay pipeline: align Nyx snapshot paths with on-disk `.mem`/`.disk`, make Lethe create usable writable layers, and clean up overlays when runs end.
- [x] Make the Firecracker runtime boot user workloads: wire command/env into VM boot, ensure rootfs uses overlay, capture guest stdout/stderr for log streaming, and return exit codes; keep Mock runtime only for tests.
- [x] Complete networking: assign IP/gateway to TAPs, plumb networking into microVMs (DHCP or static), enforce Styx contract rules, and tear down cleanly.
- [x] Add lifecycle + queue reliability: ack/nack or visibility timeout in Acheron, record run status/exit codes in Hades or a run store, and integrate Erinyes to enforce max runtime/kill on breach.
- [x] Solidify log streaming and kill: ensure agent publishes logs to Redis topics per sandbox, API streaming endpoint handles errors/timeouts, and kill commands reach the correct node.
- [x] Policy/auth baseline: relax NetworkJudge defaults or make policy configurable, require API key auth by default, and propagate tenant/user metadata for future Cerberus work.
- [x] Persist request state to Hades/Redis in Olympus (currently in-memory only).
- [x] Enforce memory limits in Erinyes (currently CPU/Runtime only).

## Observability and operations
- [x] Emit metrics (queue depth, launch latency, heartbeat age, failures) via Hermes adapters; add structured logs for control-plane events.
- [ ] Ship a dev stack (docker-compose or scripts) that boots Redis + API + agent with sane defaults for kernel/rootfs/snapshot paths and documents required host capabilities.

## Extended components backlog (design only)
- [x] Implement Cerberus authn/authz/audit gateway and replace ad-hoc bearer middleware.
- [ ] Build Charon front-door (rate limiting, circuit breaking, load-balancing) in front of Olympus.
- [ ] Add Hypnos/Thanatos lifecycle management (sleep/hibernation and graceful termination) on top of Nyx/Firecracker.
- [ ] Deliver Phlegethon/Typhon/Persephone/Kampe once core is stable: heat-based routing, quarantine pipeline, seasonal scaling, and legacy container migration adapters.
