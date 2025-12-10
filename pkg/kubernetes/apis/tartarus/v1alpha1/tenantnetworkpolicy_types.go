/*
Copyright 2025 Tartarus Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TenantNetworkPolicySpec defines the desired state of TenantNetworkPolicy
type TenantNetworkPolicySpec struct {
	// TenantID is the tenant identifier. If empty, defaults to the namespace name.
	TenantID string `json:"tenantId,omitempty"`

	// DefaultContract is the default Styx network contract for this tenant
	DefaultContract ContractSpec `json:"defaultContract,omitempty"`

	// NetworkPolicyRef references a K8s NetworkPolicy to inherit rules from
	NetworkPolicyRef *PolicyReference `json:"networkPolicyRef,omitempty"`
}

// ContractSpec defines the network contract configuration
type ContractSpec struct {
	// AllowedCIDRs are the CIDRs that sandbox traffic is allowed to reach
	AllowedCIDRs []string `json:"allowedCidrs,omitempty"`

	// DenyPrivate denies access to RFC1918 private networks (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
	DenyPrivate bool `json:"denyPrivate,omitempty"`

	// DenyMetadata denies access to cloud metadata service (169.254.169.254)
	DenyMetadata bool `json:"denyMetadata,omitempty"`
}

// PolicyReference references a K8s NetworkPolicy
type PolicyReference struct {
	// Name is the name of the NetworkPolicy
	Name string `json:"name"`

	// Namespace is the namespace of the NetworkPolicy. Defaults to the same namespace.
	Namespace string `json:"namespace,omitempty"`
}

// TenantNetworkPolicyStatus defines the observed state of TenantNetworkPolicy
type TenantNetworkPolicyStatus struct {
	// Ready indicates if the policy is ready for use
	Ready bool `json:"ready"`

	// Message provides additional details about the status
	Message string `json:"message,omitempty"`

	// LastApplied is the last time the policy was applied
	LastApplied *metav1.Time `json:"lastApplied,omitempty"`

	// Conditions represents the latest available observations of current state
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,4,rep,name=conditions"`
}

// TenantNetworkPolicyConditionType defines the type of condition
type TenantNetworkPolicyConditionType string

const (
	// TenantNetworkPolicyReady means the policy is valid and ready for use
	TenantNetworkPolicyReady TenantNetworkPolicyConditionType = "Ready"

	// TenantNetworkPolicyNetworkPolicyResolved means the referenced K8s NetworkPolicy was resolved
	TenantNetworkPolicyNetworkPolicyResolved TenantNetworkPolicyConditionType = "NetworkPolicyResolved"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// TenantNetworkPolicy is the Schema for the tenantnetworkpolicies API
type TenantNetworkPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantNetworkPolicySpec   `json:"spec,omitempty"`
	Status TenantNetworkPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TenantNetworkPolicyList contains a list of TenantNetworkPolicy
type TenantNetworkPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TenantNetworkPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TenantNetworkPolicy{}, &TenantNetworkPolicyList{})
}
