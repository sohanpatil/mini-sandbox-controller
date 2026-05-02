/*
Copyright 2026.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SandboxWarmPoolSpec defines a pool of pre-created Sandboxes.
type SandboxWarmPoolSpec struct {
	// Replicas is the desired number of warm Sandboxes.
	// +kubebuilder:validation:Minimum=0
	// +required
	Replicas int32 `json:"replicas"`

	// TemplateRef references the SandboxTemplate used to create warm Sandboxes.
	// +required
	TemplateRef SandboxTemplateRef `json:"templateRef"`
}

// SandboxWarmPoolStatus defines the observed state of SandboxWarmPool.
type SandboxWarmPoolStatus struct {
	// Replicas is the number of warm Sandboxes currently in the pool.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of warm Sandboxes whose phase is Running.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SandboxWarmPool is the Schema for the sandboxwarmpools API
type SandboxWarmPool struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of SandboxWarmPool
	// +required
	Spec SandboxWarmPoolSpec `json:"spec"`

	// status defines the observed state of SandboxWarmPool
	// +optional
	Status SandboxWarmPoolStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SandboxWarmPoolList contains a list of SandboxWarmPool
type SandboxWarmPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SandboxWarmPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SandboxWarmPool{}, &SandboxWarmPoolList{})
}
