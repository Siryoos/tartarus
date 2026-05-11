// Package themis implements the policy engine — the Goddess of Justice who
// defines and enforces the law for every tenant and template.
//
// Named after the titaness of divine law and custom, Themis stores and
// evaluates tenant-level and template-level policies that govern sandbox
// resource limits, allowed operations, and security requirements.
//
// # Core Types
//
//   - [Repository]: Interface for policy persistence (Get, Set, Delete,
//     List policies).
//
//   - [MemoryRepository]: In-memory implementation for testing and
//     development. Policies are lost on restart.
//
//   - [RedisRepository]: Durable implementation backed by Redis. Policies
//     survive restarts and are visible to all Olympus instances.
//
//   - [Policy]: Defines a set of constraints for a template:
//     max CPU, max memory, max TTL, allowed isolation types, network
//     policy reference, and annotation requirements.
//
// # Basic Usage
//
//	repo := themis.NewRedisRepository(redisClient)
//
//	// Register a policy:
//	repo.SetPolicy(ctx, &themis.Policy{
//	    TemplateID: "python3.11",
//	    MaxCPU:     4000,
//	    MaxMem:     8192,
//	    MaxTTL:     30 * time.Minute,
//	    AllowedIsolations: []domain.IsolationType{
//	        domain.IsolationMicroVM,
//	        domain.IsolationWASM,
//	    },
//	})
//
//	// In Olympus Manager.Submit:
//	policy, err := repo.GetPolicy(ctx, req.Template)
//
// # Known Technical Debt
//
//   - Policies are flat per-template records. There is no hierarchical
//     policy inheritance (e.g., tenant → team → template). Shared
//     tenant-wide defaults require duplicating policy entries for each
//     template.
//
//   - [MemoryRepository] is not concurrent-safe in all code paths;
//     the map is protected by a sync.RWMutex but Set/Delete operations
//     do not validate against concurrently running evaluations.
//
//   - Policy evaluation is a simple field comparison. Dynamic CEL-based
//     expressions (e.g., "req.cpu * req.ttl < budget") are in the roadmap
//     but not implemented.
//
//   - There is no audit trail for policy changes. An operator can modify
//     a policy in Redis without any record of who changed it or when.
//     Policy changes should flow through the hermes/audit subsystem.
package themis
