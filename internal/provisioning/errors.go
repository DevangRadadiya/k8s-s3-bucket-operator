package provisioning

import (
	"errors"
	"net"
	"strings"
)

// ProvisioningOperation indicates which MinIO provisioning step failed.
// It is used by controllers to select stable Kubernetes condition reasons
// and Prometheus "stage" label values.
type ProvisioningOperation string

const (
	OpCreateBucket    ProvisioningOperation = "create_bucket"
	OpConfigureBucket ProvisioningOperation = "configure_bucket"
	OpSetQuota        ProvisioningOperation = "set_quota"
	OpSetLifecycle    ProvisioningOperation = "set_lifecycle"
	OpGrantAccess     ProvisioningOperation = "grant_access"
	OpRevokeAccess    ProvisioningOperation = "revoke_access"
)

type ProvisioningError struct {
	Op         ProvisioningOperation
	BucketName string
	Err        error
}

func (e *ProvisioningError) Error() string {
	if e.BucketName != "" {
		return string(e.Op) + " failed for bucket " + e.BucketName + ": " + e.Err.Error()
	}
	return string(e.Op) + " failed: " + e.Err.Error()
}

func (e *ProvisioningError) Unwrap() error { return e.Err }

// IsTransientMinioError classifies backend errors as retryable.
// It is intentionally tolerant because MinIO and transport layers may wrap errors.
func IsTransientMinioError(err error) bool {
	if err == nil {
		return false
	}

	// MinIO/client errors often wrap net.Error; detect it through the error chain.
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout() || ne.Temporary()
	}

	// Fallback: match common transient substrings.
	msg := strings.ToLower(err.Error())
	transientSubstrings := []string{
		"timeout",
		"temporar",
		"connection reset",
		"connection refused",
		"broken pipe",
		"i/o timeout",
		"unexpected eof",
		"context deadline",
	}
	for _, s := range transientSubstrings {
		if strings.Contains(msg, s) {
			return true
		}
	}

	return false
}
