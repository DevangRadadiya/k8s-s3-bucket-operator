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

// PolicySourceKind identifies the Kubernetes object type containing policy content.
type PolicySourceKind string

const (
	PolicySourceKindConfigMap PolicySourceKind = "ConfigMap"
	PolicySourceKindSecret    PolicySourceKind = "Secret"
)

// BucketPolicyRef references a JSON bucket policy document stored in a ConfigMap or Secret.
// Because BucketClass is cluster-scoped, the reference must include a namespace.
type BucketPolicyRef struct {
	// Kind is either ConfigMap or Secret.
	// +kubebuilder:validation:Enum=ConfigMap;Secret
	// +optional
	Kind PolicySourceKind `json:"kind,omitempty"`

	// Namespace is the namespace containing the referenced object.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name is the name of the referenced object.
	// +optional
	Name string `json:"name,omitempty"`

	// Key is the data key in the referenced object that contains the JSON policy document.
	// For ConfigMap, this is `.data[key]`. For Secret, `.data[key]` (base64 decoded) is used.
	// +optional
	Key string `json:"key,omitempty"`
}

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

	// Backend selects the object storage implementation. Empty or MinIO uses the MinIO-compatible
	// client (default). AWS uses S3 + IAM (requires awsCredentialSecretRef).
	// +kubebuilder:validation:Enum=MinIO;AWS
	// +optional
	Backend string `json:"backend,omitempty"`

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

	// BucketPolicyRef optionally references a bucket policy JSON document to be applied when backend=AWS.
	// This is applied as an S3 bucket policy (guardrails affect all access).
	// +optional
	BucketPolicyRef *BucketPolicyRef `json:"bucketPolicyRef,omitempty"`

	// MinioCredentialSecretRef references a Secret (namespace + name) with MinIO admin credentials for this class.
	// When omitted, the operator uses MINIO_* from its own process environment (Deployment env).
	// Secret keys: MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY, and optionally MINIO_USE_SSL ("true"/"false").
	// Aliases also accepted: endpoint, accessKey, secretKey, useSSL.
	// +optional
	MinioCredentialSecretRef *corev1.SecretReference `json:"minioCredentialSecretRef,omitempty"`

	// AwsCredentialSecretRef references a Secret (namespace + name) with AWS credentials/config for this class.
	//
	// Required keys:
	// - AWS_REGION
	// - AWS_ACCESS_KEY_ID
	// - AWS_SECRET_ACCESS_KEY
	//
	// Optional keys:
	// - AWS_S3_ENDPOINT: custom S3 endpoint (for S3-compatible backends / testing)
	//
	// Aliases also accepted:
	// - region, accessKeyID, secretAccessKey, s3Endpoint
	//
	// +optional
	AwsCredentialSecretRef *corev1.SecretReference `json:"awsCredentialSecretRef,omitempty"`
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
