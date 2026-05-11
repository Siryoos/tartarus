// Package judges implements admission control — the Judges of the Dead who
// determine the fate of every sandbox request before it enters the system.
//
// Named after the three judges of the underworld (Minos, Rhadamanthus, Aeacus)
// who evaluated souls, this package evaluates incoming [domain.SandboxRequest]
// objects against a chain of admission policies:
//
//   - Quota enforcement (Minos): Are resource requests within tenant limits?
//   - Security policy (Rhadamanthus): Does the request meet security criteria?
//   - Compliance (Aeacus): Does the request satisfy regulatory/audit rules?
//
// # Core Types
//
//   - [Judge]: Single admission evaluator interface.
//
//   - [Chain]: An ordered list of [Judge] implementations that are evaluated
//     in sequence. If any judge rejects a request the chain short-circuits.
//
//   - [BasicJudge]: A simple configurable judge that checks resource limits
//     (CPU, memory, TTL) and required metadata fields.
//
//   - [Aeacus]: A compliance judge that verifies that requests carry required
//     audit and regulatory metadata.
//
// # Basic Usage
//
//	chain := judges.NewChain(
//	    judges.NewBasicJudge(judges.BasicConfig{
//	        MaxCPU: 8000, MaxMem: 16384, MaxTTL: time.Hour,
//	    }),
//	    judges.NewAeacus(complianceConfig),
//	)
//
//	if err := chain.Evaluate(ctx, req, policy); err != nil {
//	    return fmt.Errorf("admission rejected: %w", err)
//	}
//
// # Known Technical Debt
//
//   - [Aeacus] integration test ([aeacus_integration_test.go]) hits a live
//     Redis instance. There is no mock/stub for Aeacus in isolation, making
//     the test suite require Redis even for admission logic.
//
//   - The quota judge ([BasicJudge]) does not check *current* cluster usage,
//     only per-request limits. Tenant-level quota enforcement across all
//     concurrent sandboxes requires a global counter (e.g., Redis INCR with
//     TTL) which is not yet implemented.
//
//   - [Sink] (judge result routing) is defined but only logs rejections.
//     Rejected requests are not sent to an alerting sink or surfaced in
//     Prometheus metrics.
//
//   - CEL-based rule evaluation (intended for dynamic policy expressions) is
//     listed in the roadmap but not yet integrated into the chain.
package judges
