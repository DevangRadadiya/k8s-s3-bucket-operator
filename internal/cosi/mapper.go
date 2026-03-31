package cosi

import (
	"fmt"
	"regexp"
	"strings"

	v1alpha1 "github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
)

// bucketClassUIDSuffixRegex matches the COSI controller's bucket naming scheme:
// bucketName := bucketClassName + string(bucketClaim.UID) (UID is 36 chars including hyphens).
var bucketClassUIDSuffixRegex = regexp.MustCompile(`^(.+)([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// SyntheticClaimName turns a COSI bucket access "name" (typically "ba-" + UID) into a safe BucketClaim object name.
func SyntheticClaimName(cosiName string) string {
	s := strings.TrimSpace(cosiName)
	if s == "" {
		return "cosi-access"
	}
	// BucketClaim names must be valid subdomain labels; keep it simple and stable.
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// BucketClassNameFromBucketObjectName attempts to infer the BucketClassName from the COSI-generated Bucket object name.
// If inference fails, it returns empty.
func BucketClassNameFromBucketObjectName(bucketObjectName string) string {
	bucketObjectName = strings.TrimSpace(bucketObjectName)
	m := bucketClassUIDSuffixRegex.FindStringSubmatch(bucketObjectName)
	if len(m) != 3 {
		return ""
	}
	return m[1]
}

// BuildSyntheticClaim constructs a namespaced BucketClaim-like object used only to reuse provisioning helpers.
// namespace should be a stable sentinel (commonly "cosi") because COSI bucket names are globally unique in our driver.
func BuildSyntheticClaim(namespace, bucketObjectName string, parameters map[string]string) (*v1alpha1.BucketClaim, error) {
	bucketObjectName = strings.TrimSpace(bucketObjectName)
	if bucketObjectName == "" {
		return nil, fmt.Errorf("bucket object name is empty")
	}

	bucketClassName := ""
	if parameters != nil {
		bucketClassName = strings.TrimSpace(parameters["bucketClassName"])
	}
	if bucketClassName == "" {
		// Prefer decoding the UID-suffixed naming scheme used by upstream COSI.
		bucketClassName = BucketClassNameFromBucketObjectName(bucketObjectName)
	}
	if bucketClassName == "" {
		// Fallback: stable bucket names (like those used in our E2E) don't match the UID-suffix scheme.
		// We can still provision successfully because our driver mostly relies on parameters like `region`
		// (and deletionPolicy is carried separately).
		bucketClassName = "cosi-bucketclass"
	}

	claim := &v1alpha1.BucketClaim{}
	claim.Name = SyntheticClaimName(bucketObjectName)
	claim.Namespace = namespace
	claim.Spec.BucketClassName = bucketClassName
	claim.Spec.Protocols = []string{"S3"}
	claim.Spec.BucketName = strings.ToLower(bucketObjectName)
	claim.Spec.AccessType = v1alpha1.AccessTypeReadWrite

	return claim, nil
}

// BuildSyntheticClaimForAccess is like BuildSyntheticClaim, but uses the BucketAccess "name" for stable MinIO account IDs.
func BuildSyntheticClaimForAccess(namespace, bucketObjectName, accessName string, parameters map[string]string) (*v1alpha1.BucketClaim, error) {
	claim, err := BuildSyntheticClaim(namespace, bucketObjectName, parameters)
	if err != nil {
		return nil, err
	}
	claim.Name = SyntheticClaimName(accessName)
	return claim, nil
}
