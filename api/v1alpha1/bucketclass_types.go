package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeletionPolicy defines whether to Delete or Retain the bucket when the claim is deleted.
type DeletionPolicy string

const (
	DeletionPolicyDelete DeletionPolicy = "Delete"
	DeletionPolicyRetain DeletionPolicy = "Retain"
)

// RetentionMode defines default object lock retention mode.
type RetentionMode string

const (
	RetentionModeGovernance RetentionMode = "GOVERNANCE"
	RetentionModeCompliance RetentionMode = "COMPLIANCE"
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

	// ObjectLockingEnabled enables object lock during bucket creation.
	// +kubebuilder:default=false
	// +optional
	ObjectLockingEnabled bool `json:"objectLockingEnabled,omitempty"`

	// RetentionMode is the default object lock retention mode for new objects.
	// +kubebuilder:validation:Enum=GOVERNANCE;COMPLIANCE
	// +optional
	RetentionMode RetentionMode `json:"retentionMode,omitempty"`

	// RetentionDays is the default object lock retention duration in days.
	// +optional
	RetentionDays int `json:"retentionDays,omitempty"`

	// Parameters is an opaque map for the driver.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// MinioCredentialSecretRef references a Secret (namespace + name) with MinIO admin credentials for this class.
	// When omitted, the operator uses MINIO_* from its own process environment (Deployment env).
	// Secret keys: MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY, and optionally MINIO_USE_SSL ("true"/"false").
	// Aliases also accepted: endpoint, accessKey, secretKey, useSSL.
	// +optional
	MinioCredentialSecretRef *corev1.SecretReference `json:"minioCredentialSecretRef,omitempty"`
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
