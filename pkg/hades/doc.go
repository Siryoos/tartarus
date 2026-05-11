// Package hades implements the cluster node registry — the Underworld itself,
// the authoritative view of all living nodes and running sandboxes.
//
// Named after the god who rules the Underworld and tracks every soul within
// it, Hades maintains the current health, capacity, and active-sandbox list
// for every worker node in the cluster. It is the primary read source for
// the Moirai scheduler and the Olympus control plane.
//
// # Implementations
//
//   - [MemoryRegistry]: In-process registry backed by a Go sync.Map. Safe for
//     concurrent use. Does not persist across restarts; suitable for testing
//     and single-node deployments.
//
//   - [RedisRegistry]: Durable registry backed by Redis Hash and Sorted Set
//     structures. Supports TTL-based expiry to automatically deregister
//     nodes that miss heartbeats. Suitable for production multi-node clusters.
//
// # Basic Usage
//
//	reg := hades.NewRedisRegistry(redisClient, 30*time.Second)
//
//	// Agent side: heartbeat
//	reg.Register(ctx, &domain.NodeStatus{
//	    NodeInfo: domain.NodeInfo{ID: "node-1", ...},
//	    Heartbeat: time.Now(),
//	})
//
//	// Scheduler side: list available nodes
//	nodes, err := reg.ListNodes(ctx)
//
// # Known Technical Debt
//
//   - [RedisRegistry] uses a single Redis key per node (Hash). At very large
//     cluster sizes (thousands of nodes) the LIST operation becomes an O(N)
//     Redis SCAN, introducing latency spikes. Pagination or index structures
//     should be considered.
//
//   - There is no leader-election or quorum mechanism. In a split-brain
//     scenario, two Olympus instances could make divergent scheduling
//     decisions against stale Hades views.
//
//   - [ListRuns] iterates over all registered nodes to aggregate active
//     sandboxes. This is O(nodes * sandboxes) and should be replaced with a
//     dedicated index (e.g., a Redis Set of run IDs) for large deployments.
//
//   - Node labels used for affinity routing (see moirai) are not validated
//     at registration time. Malformed labels can silently break scheduling.
package hades
