// Package moirai implements the workload scheduler — the Fates who determine
// the destiny of every sandbox request by choosing its host node.
//
// Named after the three Fates (Clotho, Lachesis, Atropos) who weave, measure,
// and cut the thread of life, Moirai selects the optimal worker node for each
// sandbox request using one of several placement strategies.
//
// # Strategies
//
//   - [LeastLoadedScheduler]: Selects the node with the most free memory.
//     Simple and robust; the default strategy.
//
//   - [BinPackingScheduler]: Selects the node that is most utilised but still
//     has sufficient capacity. Reduces fragmentation and maximises node
//     utilisation at the cost of reduced headroom.
//
// # Affinity and Anti-Affinity
//
// [CheckAffinity] evaluates node label constraints expressed as metadata on
// the [domain.SandboxRequest]:
//
//	// Require the node to have label gpu=nvidia:
//	req.Metadata["scheduler.affinity.gpu"] = "nvidia"
//
//	// Exclude nodes with label environment=production:
//	req.Metadata["scheduler.antiaffinity.environment"] = "production"
//
// # Quarantine Routing
//
// Requests with metadata["quarantine"]="true" are routed exclusively to
// nodes labelled quarantine=true (Typhon nodes). See [FilterTyphonNodes].
//
// # Phlegethon Heat Routing
//
// Requests with a non-empty HeatLevel are pre-filtered to nodes that carry
// a matching Phlegethon resource class label. This enables GPU/high-memory
// nodes to be dedicated to "hot" workloads.
//
// # Basic Usage
//
//	sched := moirai.NewScheduler("least-loaded", logger)
//	nodeID, err := sched.ChooseNode(ctx, req, nodes)
//
// # Known Technical Debt
//
//   - Both scheduler strategies have O(n) node scan complexity. For very
//     large clusters this adds non-trivial latency to the scheduling path.
//     Indexed data structures (e.g., a sorted heap) would reduce this to O(log n).
//
//   - Neither strategy considers network topology (e.g., rack locality).
//     Latency-sensitive workloads may be placed on nodes far from their
//     data sources. A topology-aware strategy is needed.
//
//   - There is no re-scheduling or preemption. If a node fills up after a
//     sandbox is placed but before it launches, the launch will fail and
//     the request will be re-queued. A speculative reserve during scheduling
//     would prevent this race.
//
//   - The affinity syntax is a simple string prefix convention in
//     [domain.SandboxRequest.Metadata]. A dedicated AffinityRule struct
//     with CEL expression support would be more powerful and safer.
package moirai
