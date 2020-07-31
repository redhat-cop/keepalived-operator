package v1alpha1

import (
	"github.com/operator-framework/operator-sdk/pkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KeepalivedGroupSpec defines the desired state of KeepalivedGroup
// +k8s:openapi-gen=true
type KeepalivedGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// +kubebuilder:validation:Optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	Image string `json:"image,omitempty"`

	// +kubebuilder:validation:Required
	Interface string `json:"interface"`

	// +kubebuilder:validation:Optional
	VerbatimConfig map[string]string `json:"verbatimConfig,omitempty"`
}

// KeepalivedGroupStatus defines the observed state of KeepalivedGroup
// +k8s:openapi-gen=true
type KeepalivedGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Conditions status.Conditions `json:"conditions,omitempty"`
	RouterIDs  map[string]int    `json:"routerIDs,omitempty"`
}

func (m *KeepalivedGroup) GetReconcileStatus() status.Conditions {
	return m.Status.Conditions
}

func (m *KeepalivedGroup) SetReconcileStatus(reconcileStatus status.Conditions) {
	m.Status.Conditions = reconcileStatus
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KeepalivedGroup is the Schema for the keepalivedgroups API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=keepalivedgroups,scope=Namespaced
type KeepalivedGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeepalivedGroupSpec   `json:"spec,omitempty"`
	Status KeepalivedGroupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KeepalivedGroupList contains a list of KeepalivedGroup
type KeepalivedGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeepalivedGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeepalivedGroup{}, &KeepalivedGroupList{})
}
