// Package kubernetes provides the Kubernetes operator and custom resource
// definitions (CRDs) for managing Tartarus sandboxes as first-class K8s objects.
//
// This package allows platform teams to deploy and manage the Tartarus
// sandbox platform using standard Kubernetes tooling (kubectl, Helm, GitOps).
//
// # Sub-packages
//
//   - [kubernetes/apis/tartarus/v1alpha1]: CRD Go types for
//     [SandboxJob], [SandboxTemplate], and [TenantNetworkPolicy].
//
//   - [kubernetes/controllers]: controller-runtime reconcilers that
//     watch CRD objects and drive them to desired state via the Olympus API.
//
// # CRDs
//
// Three custom resources are defined:
//
//   - SandboxJob: Represents a single sandbox execution request. Equivalent
//     to submitting a [domain.SandboxRequest] to the Olympus API.
//
//   - SandboxTemplate: Declares a reusable execution template (base image,
//     default resources, network policy). Maps to the template registry in
//     Olympus.
//
//   - TenantNetworkPolicy: Scoped network isolation rules for a tenant.
//     Applied by Styx on the data plane.
//
// # Basic Usage
//
// Deploy the operator with the Helm chart in charts/tartarus-operator,
// then create resources with kubectl:
//
//	kubectl apply -f - <<EOF
//	apiVersion: tartarus.io/v1alpha1
//	kind: SandboxJob
//	metadata:
//	  name: my-job
//	spec:
//	  template: python3.11
//	  command: ["python", "-c", "print('hello')"]
//	  resources:
//	    cpu: 1000m
//	    memory: 512Mi
//	EOF
//
// # Known Technical Debt
//
//   - All CRDs are at v1alpha1. There is no conversion webhook, so upgrading
//     to v1beta1 will require a full migration and potential downtime.
//
//   - [SandboxJobController] does not implement status sub-resource updates
//     for all terminal states. Failed sandboxes may remain in "Pending"
//     in the K8s object until the next reconcile cycle.
//
//   - [TenantNetworkPolicyController] generates iptables rules via Styx
//     but does not handle the case where Styx is unavailable at reconcile
//     time. Policies may not be applied until the next resync period.
//
//   - There is no finaliser logic on SandboxJob. Deleting a SandboxJob
//     object while the sandbox is still running does not trigger termination;
//     the sandbox continues to run as an orphan.
//
//   - CRD validation webhooks are not implemented. Invalid specs are only
//     rejected at the controller reconcile stage, not at admission time.
package kubernetes
