/*
Copyright 2020.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KeepalivedGroupSpec defines the desired state of KeepalivedGroup
type KeepalivedGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Optional
	// +mapType=granular
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// //+kubebuilder:validation:Optional
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=registry.redhat.io/openshift4/ose-keepalived-ipfailover
	Image string `json:"image"`

	// +kubebuilder:validation:Required
	Interface string `json:"interface"`

	// +optional
	// +kubebuilder:validation:Format=ipv4
	InterfaceFromIP string `json:"interfaceFromIP"`

	// +optional
	PasswordAuth PasswordAuth `json:"passwordAuth,omitempty"`

	// +kubebuilder:validation:Optional
	// +mapType=granular
	VerbatimConfig map[string]string `json:"verbatimConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// // +kubebuilder:validation:UniqueItems=true
	// +listType=set
	BlacklistRouterIDs []int `json:"blacklistRouterIDs,omitempty"`

	// +optional
	UnicastEnabled bool `json:"unicastEnabled,omitempty"`

	// +optional
	DaemonsetPodPriorityClassName string `json:"DaemonsetPodPriorityClassName,omitempty"`

	// +kubebuilder:validation:Optional
	// +mapType=granular
	DaemonsetPodAnnotations map[string]string `json:"daemonsetPodAnnotations,omitempty"`
}

// PasswordAuth references a Kubernetes secret to extract the password for VRRP authentication
type PasswordAuth struct {
	// +required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`

	// +optional
	// +kubebuilder:default:=password
	SecretKey string `json:"secretKey"`
}

// KeepalivedGroupStatus defines the observed state of KeepalivedGroup
type KeepalivedGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// +mapType=granular
	RouterIDs map[string]int `json:"routerIDs,omitempty"`
}

func (m *KeepalivedGroup) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

func (m *KeepalivedGroup) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// KeepalivedGroup is the Schema for the keepalivedgroups API
type KeepalivedGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeepalivedGroupSpec   `json:"spec,omitempty"`
	Status KeepalivedGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeepalivedGroupList contains a list of KeepalivedGroup
type KeepalivedGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeepalivedGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeepalivedGroup{}, &KeepalivedGroupList{})
}
