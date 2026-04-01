package backend

import (
	"fmt"
	"strings"

	v1alpha1 "github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
)

// Kind identifies a storage backend implementation for a BucketClass.
type Kind string

const (
	KindMinIO Kind = "MinIO"
	KindAWS   Kind = "AWS"
)

// KindForBucketClass returns the normalized backend kind for class.
// Empty or unknown "minio" spellings default to MinIO. Unsupported values return an error.
func KindForBucketClass(class *v1alpha1.BucketClass) (Kind, error) {
	if class == nil {
		return KindMinIO, nil
	}
	b := strings.TrimSpace(class.Backend)
	if b == "" {
		return KindMinIO, nil
	}
	switch strings.ToLower(b) {
	case "minio":
		return KindMinIO, nil
	case "aws":
		return KindAWS, nil
	default:
		return "", fmt.Errorf("unsupported backend %q (supported: MinIO, AWS)", class.Backend)
	}
}
