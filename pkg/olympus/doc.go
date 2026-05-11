// Package olympus implements the Tartarus control plane — Mount Olympus,
// where the gods receive mortal requests and direct them to the Underworld.
//
// Olympus is the primary HTTP API and orchestration layer. It receives sandbox
// requests from the CLI or Kubernetes operator, runs them through admission
// control (Judges), policy checks (Themis), scheduling (Moirai), and
// enqueues them into Acheron for the Hecatoncheir agents to execute.
//
// # Core Types
//
//   - [Manager]: The main orchestrator. Validates templates, loads policies,
//     runs the judge chain, schedules to a node, and enqueues the request.
//
//   - [Scaler]: Runs a background loop that integrates with [persephone]
//     to predictively scale the cluster (pre-warm sandboxes, hibernate
//     idle nodes). Also integrates with [persephone.SeasonActivator] for
//     cron-based seasonal scaling rules.
//
//   - [ControlPlane]: Interface for sending control commands (Kill, Logs,
//     Exec, Hibernate, Wake, Snapshot) to a specific node's agent via Redis
//     Pub/Sub.
//
//   - [TemplateManager]: Manages the template registry (create, get, list,
//     delete sandbox templates).
//
//   - [Middleware]: HTTP middleware chain for auth, rate-limiting, and
//     request correlation ID injection.
//
// # Request Flow
//
//  1. HTTP request arrives at the Olympus API server.
//  2. Cerberus middleware authenticates and authorises the caller.
//  3. [Manager.Submit] validates template, loads Themis policy.
//  4. Judges evaluate the request (quota, security, compliance).
//  5. Phlegethon classifies the workload's heat level.
//  6. Moirai selects the target node.
//  7. Acheron enqueues the request.
//  8. The Hecatoncheir agent on the selected node dequeues and executes.
//
// # Known Technical Debt
//
//   - The API server uses basic net/http routing with no versioning. As
//     the API surface grows, a proper router (chi, gorilla/mux) and
//     OpenAPI schema generation are needed.
//
//   - [Manager.ListRuns] calls [hades.ListRuns] which is O(nodes * runs).
//     For large clusters this is too slow to be called on every API request.
//     A dedicated run index (Redis Sorted Set) is needed.
//
//   - Template storage is in-memory ([MemoryTemplateManager]). Templates are
//     lost on restart. A durable backend (Redis/etcd) should be wired in for
//     production.
//
//   - [Scaler] pre-warms sandboxes by calling [Manager.Submit] with synthetic
//     requests. These synthetic runs are not distinguishable from real runs
//     in metrics, inflating sandbox_submissions_total.
//
//   - The [ControlPlane] implementation ([RedisControlPlane]) publishes
//     control messages but has no delivery acknowledgement. Commands like
//     "kill" may be silently dropped if the agent is offline.
package olympus
