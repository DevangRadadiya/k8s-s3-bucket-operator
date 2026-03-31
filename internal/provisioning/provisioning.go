package provisioning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	v1alpha1 "github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/minio/minio-go/v7/pkg/replication"
	"k8s.io/apimachinery/pkg/api/resource"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// MinioClient is the subset of our MinIO wrapper the provisioning logic needs.
// It enables unit-testing without connecting to a real MinIO instance.
type MinioClient interface {
	CreateBucket(ctx context.Context, bucketName, region string, objectLocking bool) error
	SetBucketQuota(ctx context.Context, bucketName string, quotaBytes int64) error
	SetBucketLifecycle(ctx context.Context, bucketName string, cfg *lifecycle.Configuration) error
	SetBucketReplication(ctx context.Context, bucketName string, cfg replication.Config) error
	GrantAccess(ctx context.Context, bucketName, accountID, accessType, existingAccessKey, existingSecretKey string) (map[string]string, error)
	RevokeAccess(ctx context.Context, bucketName, accountID, accessKey string) error
	DeleteBucket(ctx context.Context, bucketName string) error
}

type AccessCredentials struct {
	AccessKeyID     string
	AccessSecretKey string
	BucketName      string
	Endpoint        string
}

func ProvisionBucketAndGrantAccess(
	ctx context.Context,
	mc MinioClient,
	claim *v1alpha1.BucketClaim,
	class *v1alpha1.BucketClass,
	existingAccessKey,
	existingSecretKey string,
) (AccessCredentials, error) {
	// Provision the bucket + configure it (idempotent), then grant access.
	bucketName, err := createBucketAndConfigure(ctx, mc, claim, class)
	if err != nil {
		return AccessCredentials{}, err
	}

	creds, err := grantAccess(ctx, mc, claim, bucketName, existingAccessKey, existingSecretKey)
	if err != nil {
		return AccessCredentials{}, err
	}

	return creds, nil
}

// ProvisionBucket creates/configures the bucket without granting access.
// This is useful for COSI's DriverCreateBucket handler.
func ProvisionBucket(
	ctx context.Context,
	mc MinioClient,
	claim *v1alpha1.BucketClaim,
	class *v1alpha1.BucketClass,
) (string, error) {
	return createBucketAndConfigure(ctx, mc, claim, class)
}

func createBucketAndConfigure(
	ctx context.Context,
	mc MinioClient,
	claim *v1alpha1.BucketClaim,
	class *v1alpha1.BucketClass,
) (string, error) {
	region := class.Parameters["region"]
	if region == "" {
		region = "us-east-1"
	}

	bucketName := strings.TrimSpace(claim.Spec.BucketName)
	if bucketName == "" && class.Parameters != nil {
		// COSI can pass an opaque `bucketName` hint via BucketClass.parameters.
		// When present, it takes precedence over the enterprise claim naming scheme.
		if bn := strings.TrimSpace(class.Parameters["bucketName"]); bn != "" {
			bucketName = bn
		}
	}
	if bucketName == "" {
		bucketName = fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
	}
	bucketName = strings.ToLower(bucketName)

	if err := mc.CreateBucket(ctx, bucketName, region, class.ObjectLockingEnabled); err != nil {
		return bucketName, &ProvisioningError{
			Op:         OpCreateBucket,
			BucketName: bucketName,
			Err:        err,
		}
	}

	if claim.Spec.Quota != nil {
		quotaBytes, ok := claim.Spec.Quota.AsInt64()
		if ok && quotaBytes > 0 {
			if err := mc.SetBucketQuota(ctx, bucketName, quotaBytes); err != nil {
				return bucketName, &ProvisioningError{
					Op:         OpSetQuota,
					BucketName: bucketName,
					Err:        err,
				}
			}
		}
	}

	if len(claim.Spec.LifecycleRules) > 0 {
		lc := &lifecycle.Configuration{}
		for _, rule := range claim.Spec.LifecycleRules {
			lc.Rules = append(lc.Rules, lifecycle.Rule{
				ID:     rule.ID,
				Status: rule.Status,
				RuleFilter: lifecycle.Filter{
					Prefix: rule.Prefix,
				},
				Expiration: lifecycle.Expiration{
					Days: lifecycle.ExpirationDays(rule.Expiration.Days),
				},
			})
		}
		if err := mc.SetBucketLifecycle(ctx, bucketName, lc); err != nil {
			return bucketName, &ProvisioningError{
				Op:         OpSetLifecycle,
				BucketName: bucketName,
				Err:        err,
			}
		}
	}

	// Replication is "best-effort" in our current operator behavior:
	// if it fails, keep the bucket provisioned and surface the problem in logs.
	if claim.Spec.ReplicationTarget != nil {
		repCfg := replication.Config{
			Rules: []replication.Rule{
				{
					Status: "Enabled",
					Destination: replication.Destination{
						// MinIO replication target validation expects an ARN-like destination.
						// Using `<bucket>/*` avoids "invalid ARN" errors in the demo.
						Bucket: "arn:aws:s3:::" + claim.Spec.ReplicationTarget.BucketName + "/*",
					},
				},
			},
		}

		if err := mc.SetBucketReplication(ctx, bucketName, repCfg); err != nil {
			logf.FromContext(ctx).Error(err, "Failed to set bucket replication", "bucketName", bucketName)
		}
	}

	return bucketName, nil
}

func grantAccess(
	ctx context.Context,
	mc MinioClient,
	claim *v1alpha1.BucketClaim,
	bucketName string,
	existingAccessKey,
	existingSecretKey string,
) (AccessCredentials, error) {
	accountID := fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)

	credsMap, err := mc.GrantAccess(
		ctx,
		bucketName,
		accountID,
		string(claim.Spec.AccessType),
		existingAccessKey,
		existingSecretKey,
	)
	if err != nil {
		return AccessCredentials{}, &ProvisioningError{
			Op:         OpGrantAccess,
			BucketName: bucketName,
			Err:        err,
		}
	}

	accessKeyID := strings.TrimSpace(credsMap["accessKeyID"])
	accessSecretKey := strings.TrimSpace(credsMap["accessSecretKey"])
	endpoint := strings.TrimSpace(credsMap["endpoint"])

	// Be defensive: incomplete responses must not cause silent success.
	if accessKeyID == "" || accessSecretKey == "" || endpoint == "" {
		return AccessCredentials{}, &ProvisioningError{
			Op:         OpGrantAccess,
			BucketName: bucketName,
			Err:        fmt.Errorf("minio returned incomplete credential data"),
		}
	}

	return AccessCredentials{
		AccessKeyID:     accessKeyID,
		AccessSecretKey: accessSecretKey,
		BucketName:      bucketName,
		Endpoint:        endpoint,
	}, nil
}

// GrantBucketAccess grants access to an already-provisioned bucket.
// This is used by the COSI gRPC driver (bucket creation is handled separately by COSI).
func GrantBucketAccess(
	ctx context.Context,
	mc MinioClient,
	claim *v1alpha1.BucketClaim,
	bucketName string,
	existingAccessKey,
	existingSecretKey string,
) (AccessCredentials, error) {
	return grantAccess(ctx, mc, claim, bucketName, existingAccessKey, existingSecretKey)
}

// RevokeBucketAccess revokes credentials for a bucket access identity.
// Unlike RevokeAccessAndMaybeDeleteBucket, it never deletes the bucket.
func RevokeBucketAccess(
	ctx context.Context,
	mc MinioClient,
	claim *v1alpha1.BucketClaim,
	bucketName string,
	accessKey string,
) error {
	accountID := fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
	if err := mc.RevokeAccess(ctx, bucketName, accountID, accessKey); err != nil {
		return &ProvisioningError{
			Op:         OpRevokeAccess,
			BucketName: bucketName,
			Err:        err,
		}
	}
	return nil
}

func RevokeAccessAndMaybeDeleteBucket(
	ctx context.Context,
	mc MinioClient,
	claim *v1alpha1.BucketClaim,
	class *v1alpha1.BucketClass,
	accessKey string,
) error {
	bucketName := claim.Status.BucketName
	if bucketName == "" {
		bucketName = claim.Spec.BucketName
		if bucketName == "" {
			bucketName = fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
		}
	}

	accountID := fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)

	// Always revoke access before delete/retain.
	if err := mc.RevokeAccess(ctx, bucketName, accountID, accessKey); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to revoke access during finalizer", "bucketName", bucketName)
		// Continue: MinIO deletion may still succeed.
	}

	if class.DeletionPolicy == v1alpha1.DeletionPolicyDelete {
		return mc.DeleteBucket(ctx, bucketName)
	}

	return nil
}

// ApplyClaimParameterExtensions overlays optional enterprise-style settings from an opaque
// parameters map onto a BucketClaim spec.
//
// This is primarily for COSI, where the upstream Bucket object only carries
// BucketClass.Parameters (not the namespaced BucketClaim's enterprise fields).
//
// Supported keys (all optional):
// - quota: Kubernetes quantity string (e.g. "50Gi") -> claim.Spec.Quota
// - accessType: "ReadWrite" or "ReadOnly" -> claim.Spec.AccessType
// - lifecycleRules: JSON array of v1alpha1.LifecycleRule -> claim.Spec.LifecycleRules
// - replicationTarget: JSON object v1alpha1.ReplicationTarget -> claim.Spec.ReplicationTarget
func ApplyClaimParameterExtensions(claim *v1alpha1.BucketClaim, parameters map[string]string) error {
	if claim == nil || len(parameters) == 0 {
		return nil
	}

	if q, ok := parameters["quota"]; ok && strings.TrimSpace(q) != "" {
		qty, err := resource.ParseQuantity(strings.TrimSpace(q))
		if err != nil {
			return fmt.Errorf("invalid quota quantity %q: %w", q, err)
		}
		claim.Spec.Quota = &qty
	}

	if at, ok := parameters["accessType"]; ok && strings.TrimSpace(at) != "" {
		claim.Spec.AccessType = v1alpha1.AccessType(strings.TrimSpace(at))
	}

	if lr, ok := parameters["lifecycleRules"]; ok && strings.TrimSpace(lr) != "" {
		var rules []v1alpha1.LifecycleRule
		if err := json.Unmarshal([]byte(lr), &rules); err != nil {
			return fmt.Errorf("invalid lifecycleRules JSON: %w", err)
		}
		claim.Spec.LifecycleRules = rules
	}

	if rt, ok := parameters["replicationTarget"]; ok && strings.TrimSpace(rt) != "" {
		var rep v1alpha1.ReplicationTarget
		if err := json.Unmarshal([]byte(rt), &rep); err != nil {
			return fmt.Errorf("invalid replicationTarget JSON: %w", err)
		}
		claim.Spec.ReplicationTarget = &rep
	}

	return nil
}

