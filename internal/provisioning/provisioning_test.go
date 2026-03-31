package provisioning

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	v1alpha1 "github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/minio/minio-go/v7/pkg/replication"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeMinioClient struct {
	createBucketFn       func(ctx context.Context, bucketName, region string, objectLocking bool) error
	setBucketQuotaFn     func(ctx context.Context, bucketName string, quotaBytes int64) error
	setBucketLifecycleFn func(ctx context.Context, bucketName string, cfg *lifecycle.Configuration) error
	setBucketReplicationFn func(ctx context.Context, bucketName string, cfg replication.Config) error
	grantAccessFn        func(ctx context.Context, bucketName, accountID, accessType, existingAccessKey, existingSecretKey string) (map[string]string, error)
	revokeAccessFn       func(ctx context.Context, bucketName, accountID, accessKey string) error
	deleteBucketFn       func(ctx context.Context, bucketName string) error

	createBucketArgs struct {
		bucketName    string
		region        string
		objectLocking bool
	}
}

func (f *fakeMinioClient) CreateBucket(ctx context.Context, bucketName, region string, objectLocking bool) error {
	f.createBucketArgs.bucketName = bucketName
	f.createBucketArgs.region = region
	f.createBucketArgs.objectLocking = objectLocking
	return f.createBucketFn(ctx, bucketName, region, objectLocking)
}

func (f *fakeMinioClient) SetBucketQuota(ctx context.Context, bucketName string, quotaBytes int64) error {
	return f.setBucketQuotaFn(ctx, bucketName, quotaBytes)
}

func (f *fakeMinioClient) SetBucketLifecycle(ctx context.Context, bucketName string, cfg *lifecycle.Configuration) error {
	return f.setBucketLifecycleFn(ctx, bucketName, cfg)
}

func (f *fakeMinioClient) SetBucketReplication(ctx context.Context, bucketName string, cfg replication.Config) error {
	return f.setBucketReplicationFn(ctx, bucketName, cfg)
}

func (f *fakeMinioClient) GrantAccess(ctx context.Context, bucketName, accountID, accessType, existingAccessKey, existingSecretKey string) (map[string]string, error) {
	return f.grantAccessFn(ctx, bucketName, accountID, accessType, existingAccessKey, existingSecretKey)
}

func (f *fakeMinioClient) RevokeAccess(ctx context.Context, bucketName, accountID, accessKey string) error {
	return f.revokeAccessFn(ctx, bucketName, accountID, accessKey)
}

func (f *fakeMinioClient) DeleteBucket(ctx context.Context, bucketName string) error {
	return f.deleteBucketFn(ctx, bucketName)
}

type tempNetErr struct{}

func (tempNetErr) Error() string   { return "i/o timeout" }
func (tempNetErr) Timeout() bool   { return true }
func (tempNetErr) Temporary() bool { return true }

func TestProvisionBucketAndGrantAccess_BuildsConfigsAndUsesExistingCredentials(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := resource.MustParse("100")
	claim := &v1alpha1.BucketClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
		Spec: v1alpha1.BucketClaimSpec{
			BucketClassName: "bc",
			BucketName:      "",
			Quota:           &q,
			AccessType:      v1alpha1.AccessTypeReadOnly,
			LifecycleRules: []v1alpha1.LifecycleRule{
				{
					ID:     "r1",
					Status: "Enabled",
					Prefix: "foo/",
					Expiration: v1alpha1.LifecycleExpiration{
						Days: 10,
					},
				},
			},
			ReplicationTarget: &v1alpha1.ReplicationTarget{
				Endpoint:   "http://example:9000",
				BucketName: "dst-bucket",
				AccessKey:  "a",
				SecretKey:  "b",
			},
		},
	}
	class := &v1alpha1.BucketClass{
		ObjectMeta:               metav1.ObjectMeta{Name: "bc"},
		Parameters:               map[string]string{"region": "us-west-2"},
		ObjectLockingEnabled:     true,
		DeletionPolicy:           v1alpha1.DeletionPolicyRetain,
		RetentionMode:           v1alpha1.RetentionModeGovernance,
		RetentionDays:           0,
		MinioCredentialSecretRef: &corev1.SecretReference{Name: "x", Namespace: "ns"},
	}

	existingAccessKey := "myAK"
	existingSecretKey := "mySK"

	const wantBucket = "ns1-c1"

	fake := &fakeMinioClient{}
	fake.createBucketFn = func(ctx context.Context, bucketName, region string, objectLocking bool) error {
		if bucketName != wantBucket {
			return fmt.Errorf("CreateBucket bucketName got %q want %q", bucketName, wantBucket)
		}
		if region != "us-west-2" {
			return fmt.Errorf("CreateBucket region got %q want %q", region, "us-west-2")
		}
		if !objectLocking {
			return fmt.Errorf("CreateBucket objectLocking got false want true")
		}
		return nil
	}
	fake.setBucketQuotaFn = func(ctx context.Context, bucketName string, quotaBytes int64) error {
		if bucketName != wantBucket {
			return fmt.Errorf("SetBucketQuota bucketName got %q want %q", bucketName, wantBucket)
		}
		if quotaBytes != 100 {
			return fmt.Errorf("SetBucketQuota quotaBytes got %d want %d", quotaBytes, 100)
		}
		return nil
	}
	fake.setBucketLifecycleFn = func(ctx context.Context, bucketName string, cfg *lifecycle.Configuration) error {
		if bucketName != wantBucket {
			return fmt.Errorf("SetBucketLifecycle bucketName got %q want %q", bucketName, wantBucket)
		}
		if len(cfg.Rules) != 1 {
			return fmt.Errorf("SetBucketLifecycle rules len got %d want 1", len(cfg.Rules))
		}
		if cfg.Rules[0].RuleFilter.Prefix != "foo/" {
			return fmt.Errorf("SetBucketLifecycle prefix got %q want %q", cfg.Rules[0].RuleFilter.Prefix, "foo/")
		}
		if int(cfg.Rules[0].Expiration.Days) != 10 {
			return fmt.Errorf("SetBucketLifecycle days got %d want %d", cfg.Rules[0].Expiration.Days, 10)
		}
		return nil
	}
	fake.setBucketReplicationFn = func(ctx context.Context, bucketName string, cfg replication.Config) error {
		if bucketName != wantBucket {
			return fmt.Errorf("SetBucketReplication bucketName got %q want %q", bucketName, wantBucket)
		}
		if len(cfg.Rules) != 1 {
			return fmt.Errorf("SetBucketReplication rules len got %d want 1", len(cfg.Rules))
		}
		wantARN := "arn:aws:s3:::" + "dst-bucket" + "/*"
		if cfg.Rules[0].Destination.Bucket != wantARN {
			return fmt.Errorf("SetBucketReplication destination ARN got %q want %q", cfg.Rules[0].Destination.Bucket, wantARN)
		}
		return nil
	}
	fake.grantAccessFn = func(ctx context.Context, bucketName, accountID, accessType, existingAK, existingSK string) (map[string]string, error) {
		if bucketName != wantBucket {
			return nil, fmt.Errorf("GrantAccess bucketName got %q want %q", bucketName, wantBucket)
		}
		if accountID != wantBucket {
			return nil, fmt.Errorf("GrantAccess accountID got %q want %q", accountID, wantBucket)
		}
		if accessType != string(v1alpha1.AccessTypeReadOnly) {
			return nil, fmt.Errorf("GrantAccess accessType got %q want %q", accessType, string(v1alpha1.AccessTypeReadOnly))
		}
		if existingAK != existingAccessKey {
			return nil, fmt.Errorf("GrantAccess existingAccessKey got %q want %q", existingAK, existingAccessKey)
		}
		if existingSK != existingSecretKey {
			return nil, fmt.Errorf("GrantAccess existingSecretKey got %q want %q", existingSK, existingSecretKey)
		}
		return map[string]string{
			"accessKeyID":     existingAK,
			"accessSecretKey": existingSK,
			"bucketName":      bucketName,
			"endpoint":        "http://minio:9000",
		}, nil
	}
	fake.revokeAccessFn = func(ctx context.Context, bucketName, accountID, accessKey string) error { return nil }
	fake.deleteBucketFn = func(ctx context.Context, bucketName string) error { return nil }

	creds, err := ProvisionBucketAndGrantAccess(ctx, fake, claim, class, existingAccessKey, existingSecretKey)
	if err != nil {
		t.Fatalf("ProvisionBucketAndGrantAccess error: %v", err)
	}
	if creds.AccessKeyID != existingAccessKey || creds.AccessSecretKey != existingSecretKey {
		t.Fatalf("returned creds mismatch: got %q/%q want %q/%q", creds.AccessKeyID, creds.AccessSecretKey, existingAccessKey, existingSecretKey)
	}
	if creds.BucketName != wantBucket {
		t.Fatalf("returned bucketName got %q want %q", creds.BucketName, wantBucket)
	}
	if creds.Endpoint != "http://minio:9000" {
		t.Fatalf("returned endpoint got %q want %q", creds.Endpoint, "http://minio:9000")
	}
}

func TestProvisionBucketAndGrantAccess_ReplicationBestEffort(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	claim := &v1alpha1.BucketClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
		Spec:       v1alpha1.BucketClaimSpec{BucketClassName: "bc", ReplicationTarget: &v1alpha1.ReplicationTarget{BucketName: "dst"}},
	}
	class := &v1alpha1.BucketClass{
		ObjectMeta:           metav1.ObjectMeta{Name: "bc"},
		Parameters:           map[string]string{},
		ObjectLockingEnabled: false,
	}

	fake := &fakeMinioClient{}
	fake.createBucketFn = func(ctx context.Context, bucketName, region string, objectLocking bool) error { return nil }
	fake.setBucketQuotaFn = func(ctx context.Context, bucketName string, quotaBytes int64) error { return nil }
	fake.setBucketLifecycleFn = func(ctx context.Context, bucketName string, cfg *lifecycle.Configuration) error { return nil }
	fake.setBucketReplicationFn = func(ctx context.Context, bucketName string, cfg replication.Config) error {
		return errors.New("replication failed")
	}
	fake.grantAccessFn = func(ctx context.Context, bucketName, accountID, accessType, existingAK, existingSK string) (map[string]string, error) {
		return map[string]string{
			"accessKeyID":     "ak",
			"accessSecretKey": "sk",
			"bucketName":      bucketName,
			"endpoint":        "http://minio:9000",
		}, nil
	}
	fake.revokeAccessFn = func(ctx context.Context, bucketName, accountID, accessKey string) error { return nil }
	fake.deleteBucketFn = func(ctx context.Context, bucketName string) error { return nil }

	if _, err := ProvisionBucketAndGrantAccess(ctx, fake, claim, class, "", ""); err != nil {
		t.Fatalf("expected replication errors to be best-effort, got: %v", err)
	}
}

func TestIsTransientMinioError_WrappedNetError(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("outer context: %w", tempNetErr{})
	if !IsTransientMinioError(wrapped) {
		t.Fatalf("expected wrapped net.Error to be treated as transient")
	}
}

func TestProvisionBucketAndGrantAccess_TransientClassifiedByOp(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	claim := &v1alpha1.BucketClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
		Spec:       v1alpha1.BucketClaimSpec{BucketClassName: "bc"},
	}
	class := &v1alpha1.BucketClass{ObjectMeta: metav1.ObjectMeta{Name: "bc"}, Parameters: map[string]string{}}

	fake := &fakeMinioClient{}
	fake.createBucketFn = func(ctx context.Context, bucketName, region string, objectLocking bool) error {
		return fmt.Errorf("wrap: %w", tempNetErr{})
	}
	fake.setBucketQuotaFn = func(ctx context.Context, bucketName string, quotaBytes int64) error { return nil }
	fake.setBucketLifecycleFn = func(ctx context.Context, bucketName string, cfg *lifecycle.Configuration) error { return nil }
	fake.setBucketReplicationFn = func(ctx context.Context, bucketName string, cfg replication.Config) error { return nil }
	fake.grantAccessFn = func(ctx context.Context, bucketName, accountID, accessType, existingAK, existingSK string) (map[string]string, error) {
		return nil, errors.New("should not be called")
	}
	fake.revokeAccessFn = func(ctx context.Context, bucketName, accountID, accessKey string) error { return nil }
	fake.deleteBucketFn = func(ctx context.Context, bucketName string) error { return nil }

	_, err := ProvisionBucketAndGrantAccess(ctx, fake, claim, class, "", "")
	if err == nil {
		t.Fatalf("expected error")
	}

	var pErr *ProvisioningError
	if !errors.As(err, &pErr) {
		t.Fatalf("expected ProvisioningError, got %T", err)
	}
	if pErr.Op != OpCreateBucket {
		t.Fatalf("Op: got %q want %q", pErr.Op, OpCreateBucket)
	}
	if pErr.BucketName != "ns1-c1" {
		t.Fatalf("BucketName: got %q want %q", pErr.BucketName, "ns1-c1")
	}
	if !IsTransientMinioError(err) {
		t.Fatalf("expected provisioning error to be classified as transient")
	}
}

// Compile-time check: fakeMinioClient must implement MinioClient.
var _ MinioClient = (*fakeMinioClient)(nil)

// net imports are referenced by the type's methods; keep it explicit for clarity.
var _ net.Error = (*tempNetErr)(nil)

