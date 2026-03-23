package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeletionPolicy defines whether to Delete or Retain the bucket when the claim is deleted.
type DeletionPolicy string

const (
	DeletionPolicyDelete DeletionPolicy = "Delete"
	DeletionPolicyRetain DeletionPolicy = "Retain"
)

// BucketClass defines the parameters for a class of buckets.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Driver",type="string",JSONPath=".driverName"
// +kubebuilder:printcolumn:name="DeletionPolicy",type="string",JSONPath=".deletionPolicy"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type BucketClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// DriverName specifies which operator should handle claims using this class.
	// +kubebuilder:validation:Required
	DriverName string `json:"driverName"`

	// DeletionPolicy is either Delete or Retain.
	// +kubebuilder:validation:Enum=Delete;Retain
	// +kubebuilder:default=Retain
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

	// Parameters is an opaque map for the driver.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`
}

// +kubebuilder:object:root=true

// BucketClassList contains a list of BucketClass
type BucketClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BucketClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BucketClass{}, &BucketClassList{})
}
