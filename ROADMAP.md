# Tartarus Roadmap (Reality)

## Implementation snapshot (Nov 2024)
| Component | Status | Reality |
| --- | --- | --- |
| Olympus API (`cmd/olympus-api`, `pkg/olympus`) | Partial | Accepts submit/list/kill/logs; no persistence of runs, no scheduler dispatch, single-region only; auth is optional bearer env var. |
| Control plane (kill/log pubsub) | Partial | Redis pubsub path exists; run state is not persisted and API cannot see nodes unless everything runs in one process. |
| Scheduler & placement (`pkg/moirai`) | Unused | Scheduler exists but `Manager.Submit` never calls it; requests are blindly enqueued with no node assignment or capacity checks. |
| Acheron queue | Basic | In-memory/Redis queues work but have no visibility timeout, ack/retry, or dead-letter; agent never ACKs. |
| Hecatoncheir agent | Partial | Dequeues and launches but uses mock runtime by default; does not run judges/furies, does not update registry capacity, and leaves queue items unacked. |
| Runtime (`pkg/tartarus`) | Partial | Firecracker runtime is present but assumes valid rootfs/kernel and lacks user command wiring; log streaming tails the Firecracker log, not guest stdout. |
| Nyx snapshots & Lethe overlays | Broken path | Nyx Prepare is never called from the agent; snapshot.Path points to a base name while Lethe copies it directly (no .disk/.img), so overlay creation fails. |
| Styx networking | Partial | Creates bridge/TAP and iptables rules but does not assign IPs to taps or plumb networking into microVMs; no policy-aware rules. |
| Policies/Judges/Erinyes | Minimal | Default lockdown policy only; NetworkJudge rejects most requests; furies are constructed but never armed; no policy persistence APIs. |
| Registry (`pkg/hades`) | Partial | Memory/Redis implementations exist; agent currently uses memory registry only, so the API cannot discover nodes in multi-process setups. |
| Observability | Minimal | Slog adapters and noop metrics only; no dashboards, tracing, or audit logging. |
| Extended components (Cerberus, Charon, Hypnos, Thanatos, Persephone, Phlegethon, Typhon, Kampe) | Design only | No packages exist beyond docs. |

## Milestones
- **MVP 0.1 (single node)**: end-to-end sandbox run with real snapshot/overlay, Firecracker runtime, functional networking, log streaming, kill, and required API key auth.
- **0.2 Multi-node control plane**: shared registry via Redis, scheduler-driven placement, targeted dispatch to agents, run state persistence, and reliable queue semantics.
- **Phase 4-6 extensions**: implement Cerberus/Charon/Hypnos/Thanatos/Persephone/Phlegethon/Typhon/Kampe per design once core is stable (see `extended-components.md` and `TASKS.md`).
