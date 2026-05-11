// Package typhon implements security hardening and the quarantine pool — the
// Monster of Chaos who contains the most dangerous workloads.
//
// Named after the most fearsome monster in Greek mythology, Typhon manages
// two related security concerns: seccomp profile generation for guest kernels,
// and runtime quarantine of sandboxes that exhibit suspicious behaviour.
//
// # Core Types
//
//   - [Classifier]: Evaluates a [domain.SandboxRequest] against configurable
//     CEL-expression triggers and determines if it should be quarantined.
//
//   - [RuleBasedClassifier]: Production implementation of [Classifier] using
//     Google's CEL library. Evaluates CPU, memory, template, metadata, and
//     network traffic against auto-quarantine trigger rules.
//
//   - [QuarantinePool]: Manages the set of currently quarantined sandboxes.
//     Supports adding, releasing, listing, and querying sandbox records.
//
//   - [HardenedManager]: Wraps a [tartarus.SandboxRuntime] and applies
//     additional hardening steps (seccomp profile, read-only rootfs,
//     restricted capabilities) when [domain.SandboxRequest.Hardened] is set.
//
//   - [SeccompProfileGenerator]: Generates seccomp profiles for guest kernels,
//     either from built-in templates (strict, default, permissive) or by
//     learning from strace output via [AnalyzeStrace].
//
//   - [StraceAnalyzer]: Parses strace output to extract the set of syscalls
//     made by a process and produces a minimal-allow-list seccomp profile.
//
// # Quarantine Flow
//
//  1. Hecatoncheir calls [Classifier.ShouldQuarantine] before launching.
//  2. If quarantined, the request is tagged metadata["quarantine"]="true".
//  3. Moirai routes it to Typhon-labelled nodes (quarantine=true).
//  4. [QuarantinePool] records the sandbox with evidence and reason.
//  5. Operators can review and release via the CLI.
//
// # Seccomp Profile Usage
//
//	gen := typhon.NewSeccompProfileGenerator()
//	profile, err := gen.GenerateFromTemplate(typhon.TemplateStrict)
//
//	// Or learn from strace:
//	syscalls, err := typhon.AnalyzeStrace(ctx, "/tmp/strace.log")
//	profile, err = gen.GenerateFromSyscalls(syscalls)
//
// # Known Technical Debt
//
//   - [RuleBasedClassifier] evaluates network_egress and network_ingress
//     as zero-valued placeholders. Actual per-sandbox network traffic
//     metrics from Erinyes are not wired in, so network-based triggers
//     never fire.
//
//   - [SeccompProfileGenerator] template files are not embedded in the
//     binary (no go:embed). They must exist at the path specified during
//     construction. Deployments must ship these files separately.
//
//   - [HardenedManager] applies hardening at launch but does not enforce
//     hardening during the sandbox's lifetime (e.g., does not prevent
//     in-flight capability changes inside the VM).
//
//   - CEL expressions in [AutoQuarantineTrigger] are compiled on every
//     evaluation call. These should be compiled once at configuration
//     load time and cached for performance.
package typhon
