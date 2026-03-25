package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessType defines the type of generated bucket access.
type AccessType string

const (
	AccessTypeReadWrite AccessType = "ReadWrite"
	AccessTypeReadOnly  AccessType = "ReadOnly"
)

// LifecycleExpiration defines expiration settings for a lifecycle rule.
type LifecycleExpiration struct {
	// Days is the object expiration period in days.
	Days int `json:"days"`
}

// LifecycleRule defines one lifecycle management rule.
type LifecycleRule struct {
	// ID is the unique identifier for the rule.
	ID string `json:"id"`
	// +kubebuilder:validation:Enum=Enabled;Disabled
	Status string `json:"status"`
	// Prefix limits the rule to object keys with this prefix.
	// +optional
	Prefix string `json:"prefix,omitempty"`
	// Expiration defines the expiration policy for matched objects.
	Expiration LifecycleExpiration `json:"expiration"`
}

// ReplicationTarget defines the target MinIO deployment for replication.
type ReplicationTarget struct {
	// Endpoint is the target MinIO API endpoint (host:port).
	Endpoint string `json:"endpoint"`
	// BucketName is the destination bucket name in the target deployment.
	BucketName string `json:"bucketName"`
	// AccessKey is the admin/service access key for the target deployment.
	AccessKey string `json:"accessKey"`
	// SecretKey is the admin/service secret key for the target deployment.
	SecretKey string `json:"secretKey"`
	// UseSSL controls whether the target endpoint uses TLS.
	// +optional
	UseSSL bool `json:"useSSL,omitempty"`
}

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

	// Quota sets a hard storage quota for the bucket.
	// +optional
	Quota *resource.Quantity `json:"quota,omitempty"`

	// AccessType controls generated IAM access.
	// +kubebuilder:validation:Enum=ReadWrite;ReadOnly
	// +kubebuilder:default=ReadWrite
	// +optional
	AccessType AccessType `json:"accessType,omitempty"`

	// LifecycleRules defines object lifecycle management rules.
	// +optional
	LifecycleRules []LifecycleRule `json:"lifecycleRules,omitempty"`

	// ReplicationTarget enables configuring replication to another MinIO deployment.
	// +optional
	ReplicationTarget *ReplicationTarget `json:"replicationTarget,omitempty"`
}

// BucketClaimStatus defines the observed state of BucketClaim
type BucketClaimStatus struct {
	// Conditions represent the latest available observations of the claim's state.
	// The Ready condition surfaces provisioning progress (Provisioning), success (BucketProvisioned), or failure reasons.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SecretReference points to the Secret containing the bucket credentials.
	// +optional
	SecretReference *v1.ObjectReference `json:"secretReference,omitempty"`

	// Endpoint is the S3 endpoint URL.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// BucketName is the actual name of the created bucket.
	// +optional
	BucketName string `json:"bucketName,omitempty"`

	// Phase represents the current state of the BucketClaim (e.g. Pending, Bound, Failed).
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
