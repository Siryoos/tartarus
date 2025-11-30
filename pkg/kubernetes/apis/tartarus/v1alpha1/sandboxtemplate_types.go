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

// SandboxTemplateSpec defines the desired state of SandboxTemplate
type SandboxTemplateSpec struct {
	// Image is the OCI image to use as a base
	Image string `json:"image"`

	// DefaultCommand is the default command to run if none is specified
	DefaultCommand []string `json:"defaultCommand,omitempty"`

	// DefaultEnv defines default environment variables
	DefaultEnv map[string]string `json:"defaultEnv,omitempty"`

	// DefaultResources defines the default resource requirements
	DefaultResources ResourceSpec `json:"defaultResources,omitempty"`

	// DefaultNetwork defines the default network policy
	DefaultNetwork NetworkPolicyRef `json:"defaultNetwork,omitempty"`

	// DefaultHeatLevel defines the default heat level
	DefaultHeatLevel string `json:"defaultHeatLevel,omitempty"`

	// DefaultRetention defines the default retention policy
	DefaultRetention RetentionPolicy `json:"defaultRetention,omitempty"`
}

// SandboxTemplateStatus defines the observed state of SandboxTemplate
type SandboxTemplateStatus struct {
	// Ready indicates if the template is ready for use
	Ready bool `json:"ready"`

	// Message provides additional details about the status
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SandboxTemplate is the Schema for the sandboxtemplates API
type SandboxTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SandboxTemplateSpec   `json:"spec,omitempty"`
	Status SandboxTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SandboxTemplateList contains a list of SandboxTemplate
type SandboxTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SandboxTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SandboxTemplate{}, &SandboxTemplateList{})
}
