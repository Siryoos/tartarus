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

// SandboxJobSpec defines the desired state of SandboxJob
type SandboxJobSpec struct {
	// Template is the ID of the template to use (e.g. "alpine-base")
	Template string `json:"template"`

	// Command is the command to run in the sandbox
	Command []string `json:"command"`

	// Args are the arguments to the command
	Args []string `json:"args,omitempty"`

	// Env defines environment variables
	Env map[string]string `json:"env,omitempty"`

	// Resources defines the resource requirements
	Resources ResourceSpec `json:"resources,omitempty"`

	// Network defines the network policy reference
	Network NetworkPolicyRef `json:"network,omitempty"`

	// HeatLevel defines the Phlegethon heat classification (e.g. "ember", "inferno")
	HeatLevel string `json:"heatLevel,omitempty"`

	// Retention defines the retention policy
	Retention RetentionPolicy `json:"retention,omitempty"`

	// Metadata defines arbitrary metadata (tenant, user, etc.)
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NetworkPolicyRef defines a reference to a network policy
type NetworkPolicyRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// RetentionPolicy defines the retention policy for the sandbox
type RetentionPolicy struct {
	// MaxAge is the maximum age of the sandbox
	MaxAge string `json:"maxAge,omitempty"` // Duration string, e.g. "1h"

	// KeepOutputs indicates whether to keep the outputs after the sandbox is gone
	KeepOutputs bool `json:"keepOutputs,omitempty"`
}

// ResourceSpec defines the resources for the sandbox
type ResourceSpec struct {
	// CPU millicores (e.g. 1000 = 1 core)
	CPU int `json:"cpu,omitempty"`

	// Memory in Megabytes
	Memory int `json:"memory,omitempty"`
}

// SandboxJobStatus defines the observed state of SandboxJob
type SandboxJobStatus struct {
	// ID is the Tartarus Sandbox ID
	ID string `json:"id,omitempty"`

	// State is the current state of the sandbox
	State string `json:"state,omitempty"`

	// NodeID is the ID of the node running the sandbox
	NodeID string `json:"nodeID,omitempty"`

	// Message provides additional details about the status
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SandboxJob is the Schema for the sandboxjobs API
type SandboxJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SandboxJobSpec   `json:"spec,omitempty"`
	Status SandboxJobStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SandboxJobList contains a list of SandboxJob
type SandboxJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SandboxJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SandboxJob{}, &SandboxJobList{})
}
