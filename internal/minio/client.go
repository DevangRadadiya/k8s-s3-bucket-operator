package minio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"k8s.io/klog/v2"
)

// Config holds connection details for a MinIO instance.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// ConfigFromEnv reads MinIO connection details from environment variables.
//
//	MINIO_ENDPOINT    e.g. "minio.minio-ns.svc.cluster.local:9000"
//	MINIO_ACCESS_KEY  admin access key
//	MINIO_SECRET_KEY  admin secret key
//	MINIO_USE_SSL     "true" or "false" (default: false)
func ConfigFromEnv() Config {
	useSSL := os.Getenv("MINIO_USE_SSL") == "true"
	return Config{
		Endpoint:  os.Getenv("MINIO_ENDPOINT"),
		AccessKey: os.Getenv("MINIO_ACCESS_KEY"),
		SecretKey: os.Getenv("MINIO_SECRET_KEY"),
		UseSSL:    useSSL,
	}
}

// Client wraps both the MinIO S3 client (bucket operations)
// and the MinIO admin client (user + policy management).
type Client struct {
	s3    *miniogo.Client
	admin *madmin.AdminClient
}

// NewClient creates a new MinIO client pair from the given config.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("MINIO_ENDPOINT is required")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY are required")
	}

	s3, err := miniogo.New(cfg.Endpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	admin, err := madmin.New(cfg.Endpoint, cfg.AccessKey, cfg.SecretKey, cfg.UseSSL)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin client: %w", err)
	}

	return &Client{s3: s3, admin: admin}, nil
}

// CreateBucket creates a bucket with the given name and region.
// Returns nil if the bucket already exists.
func (c *Client) CreateBucket(ctx context.Context, bucketName, region string) error {
	exists, err := c.s3.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("checking bucket existence: %w", err)
	}
	if exists {
		klog.V(4).Infof("Bucket %q already exists, skipping creation", bucketName)
		return nil
	}

	opts := miniogo.MakeBucketOptions{Region: region}
	if err := c.s3.MakeBucket(ctx, bucketName, opts); err != nil {
		return fmt.Errorf("creating bucket %q: %w", bucketName, err)
	}

	klog.Infof("Created bucket %q in region %q", bucketName, region)
	return nil
}

// DeleteBucket removes a bucket. Does not fail if the bucket does not exist.
func (c *Client) DeleteBucket(ctx context.Context, bucketName string) error {
	exists, err := c.s3.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("checking bucket existence: %w", err)
	}
	if !exists {
		klog.V(4).Infof("Bucket %q not found, skipping deletion", bucketName)
		return nil
	}

	if err := c.s3.RemoveBucket(ctx, bucketName); err != nil {
		return fmt.Errorf("deleting bucket %q: %w", bucketName, err)
	}

	klog.Infof("Deleted bucket %q", bucketName)
	return nil
}

// GrantAccess creates a MinIO user scoped to the given bucket and returns credentials.
// The returned map contains: accessKeyID, accessSecretKey, bucketName, endpoint.
func (c *Client) GrantAccess(ctx context.Context, bucketName, accountID string) (map[string]string, error) {
	accessKey, secretKey, err := generateCredentials()
	if err != nil {
		return nil, fmt.Errorf("generating credentials: %w", err)
	}

	// Create the user
	if err := c.admin.AddUser(ctx, accessKey, secretKey); err != nil {
		return nil, fmt.Errorf("creating MinIO user %q: %w", accessKey, err)
	}

	// Create a bucket-scoped policy
	policyName := fmt.Sprintf("cosi-%s-%s", bucketName, accountID)
	policy := bucketPolicy(bucketName)

	policyBytes, err := json.Marshal(policy)
	if err != nil {
		return nil, fmt.Errorf("marshalling bucket policy: %w", err)
	}

	if err := c.admin.AddCannedPolicy(ctx, policyName, policyBytes); err != nil {
		return nil, fmt.Errorf("creating policy %q: %w", policyName, err)
	}

	// Attach policy to user
	if err := c.admin.SetPolicy(ctx, policyName, accessKey, false); err != nil {
		return nil, fmt.Errorf("attaching policy to user %q: %w", accessKey, err)
	}

	klog.Infof("Granted access to bucket %q for account %q (user: %s)", bucketName, accountID, accessKey)

	return map[string]string{
		"accessKeyID":     accessKey,
		"accessSecretKey": secretKey,
		"bucketName":      bucketName,
		"endpoint":        c.s3.EndpointURL().String(),
	}, nil
}

// RevokeAccess removes the MinIO user and associated policy for the given account.
func (c *Client) RevokeAccess(ctx context.Context, bucketName, accountID string) error {
	policyName := fmt.Sprintf("cosi-%s-%s", bucketName, accountID)

	// Remove user (accountID is used as the accessKey in this operator)
	if err := c.admin.RemoveUser(ctx, accountID); err != nil {
		klog.Warningf("Could not remove user %q: %v (may already be deleted)", accountID, err)
	}

	// Remove policy
	if err := c.admin.RemoveCannedPolicy(ctx, policyName); err != nil {
		klog.Warningf("Could not remove policy %q: %v (may already be deleted)", policyName, err)
	}

	klog.Infof("Revoked access to bucket %q for account %q", bucketName, accountID)
	return nil
}

// generateCredentials creates a random access key and secret key.
func generateCredentials() (accessKey, secretKey string, err error) {
	akBytes := make([]byte, 10)
	skBytes := make([]byte, 20)

	if _, err = rand.Read(akBytes); err != nil {
		return
	}
	if _, err = rand.Read(skBytes); err != nil {
		return
	}

	accessKey = "cosi-" + hex.EncodeToString(akBytes)
	secretKey = hex.EncodeToString(skBytes)
	return
}

// bucketPolicy returns an IAM policy document scoped to a single bucket.
func bucketPolicy(bucketName string) map[string]interface{} {
	return map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"s3:GetObject",
					"s3:PutObject",
					"s3:DeleteObject",
					"s3:ListBucket",
					"s3:GetBucketLocation",
				},
				"Resource": []string{
					fmt.Sprintf("arn:aws:s3:::%s", bucketName),
					fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
				},
			},
		},
	}
}
