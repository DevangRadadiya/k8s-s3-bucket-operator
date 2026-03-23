package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BucketClaimSpec defines the desired state of BucketClaim
type BucketClaimSpec struct {
	// BucketClassName is the name of the BucketClass to use.
	// +kubebuilder:validation:Required
	BucketClassName string `json:"bucketClassName"`

	// Protocols is a list of object storage protocols. For this MVP, only 'S3' is expected.
	// +optional
	Protocols []string `json:"protocols,omitempty"`

	// BucketName allows explicitly naming the bucket. If empty, the operator generates one.
	// +optional
	BucketName string `json:"bucketName,omitempty"`
}

// BucketClaimStatus defines the observed state of BucketClaim
type BucketClaimStatus struct {
	// SecretReference points to the Secret containing the bucket credentials.
	// +optional
	SecretReference *v1.ObjectReference `json:"secretReference,omitempty"`

	// Endpoint is the S3 endpoint URL.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// BucketName is the actual name of the created bucket.
	// +optional
	BucketName string `json:"bucketName,omitempty"`

	// Phase represents the current state of the BucketClaim (e.g. Bound, Pending).
	// +optional
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Class",type="string",JSONPath=".spec.bucketClassName"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Secret",type="string",JSONPath=".status.secretReference.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BucketClaim is the Schema for the bucketclaims API
type BucketClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BucketClaimSpec   `json:"spec,omitempty"`
	Status BucketClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BucketClaimList contains a list of BucketClaim
type BucketClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BucketClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BucketClaim{}, &BucketClaimList{})
}
