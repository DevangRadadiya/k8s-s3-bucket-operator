package backend

import (
	"context"

	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/minio/minio-go/v7/pkg/replication"
)

// Provider is the object storage surface used by provisioning (bucket lifecycle, access, delete).
// Implementations include the in-repo MinIO client and (planned) AWS S3.
type Provider interface {
	CreateBucket(ctx context.Context, bucketName, region string, objectLocking bool) error
	SetBucketQuota(ctx context.Context, bucketName string, quotaBytes int64) error
	SetBucketLifecycle(ctx context.Context, bucketName string, cfg *lifecycle.Configuration) error
	SetBucketReplication(ctx context.Context, bucketName string, cfg replication.Config) error
	GrantAccess(ctx context.Context, bucketName, accountID, accessType, existingAccessKey, existingSecretKey string) (map[string]string, error)
	RevokeAccess(ctx context.Context, bucketName, accountID, accessKey string) error
	DeleteBucket(ctx context.Context, bucketName string) error
}

// BucketSecurityConfigurer is an optional interface that backends can implement to apply
// backend-specific bucket security configuration (encryption, public access blocks, etc.).
//
// parameters is typically BucketClass.spec.parameters.
type BucketSecurityConfigurer interface {
	ConfigureBucketSecurity(ctx context.Context, bucketName string, parameters map[string]string) error
}
