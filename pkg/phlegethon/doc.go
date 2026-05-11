// Package phlegethon implements hot-path routing — the River of Fire that
// directs high-demand workloads to pre-warmed, resource-class-matched nodes.
//
// Named after the river of fire in Hades, Phlegethon classifies each incoming
// sandbox request by its "heat" level — a proxy for the intensity of its
// resource requirements and latency expectations — and routes it to nodes
// that have been pre-warmed for that class of workload.
//
// # Core Types
//
//   - [HeatClassifier]: Classifies a [domain.SandboxRequest] into a heat
//     level (e.g., "cold", "warm", "hot") based on CPU, memory, TTL, and
//     workload metadata.
//
//   - [Router]: Routes classified requests to the appropriate Moirai
//     scheduling tier (standard nodes vs. pre-warmed Phlegethon nodes).
//
// # Heat Levels
//
//   - "cold"  : Infrequently run, no snapshot available. Standard node pool.
//   - "warm"  : Moderate frequency, snapshot may be available. Nyx pre-warmed pool.
//   - "hot"   : High-frequency, snapshot guaranteed. Dedicated Phlegethon nodes.
//
// # Basic Usage
//
//	classifier := phlegethon.NewHeatClassifier(phlegethon.Config{
//	    HotCPUThreshold:  4000, // milliCPU
//	    HotMemThreshold:  4096, // MB
//	})
//	heat := classifier.Classify(req)
//	req.HeatLevel = heat
//
// # Known Technical Debt
//
//   - Heat classification relies solely on static request attributes
//     (CPU, memory). Dynamic signals (recent launch frequency for this
//     template, queue depth) are not yet incorporated. A feedback loop
//     from Persephone would make classification more accurate.
//
//   - The "warm" tier uses a snapshot if one exists but does not pre-create
//     missing snapshots. An explicit pre-warm API (trigger Nyx to snapshot
//     a template) is needed for warm-tier SLA guarantees.
//
//   - Node labelling for Phlegethon resource classes is done manually by
//     the operator. There is no auto-labelling mechanism based on node
//     hardware capabilities (e.g., GPU presence, NVMe storage).
//
//   - The routing logic in [Router] is not integrated with Charon's
//     load-balancing strategies. Phlegethon and Charon operate
//     independently, which can lead to conflicting placement decisions.
package phlegethon
