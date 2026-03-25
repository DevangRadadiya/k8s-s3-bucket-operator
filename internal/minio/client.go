package minio

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/minio/minio-go/v7/pkg/replication"
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

func firstSecretString(data map[string][]byte, keys ...string) string {
	for _, k := range keys {
		if v, ok := data[k]; ok && len(v) > 0 {
			return strings.TrimSpace(string(v))
		}
	}
	return ""
}

// ConfigFromSecretData builds MinIO config from Secret .Data (same keys as operator env).
// Accepts MINIO_* keys or aliases: endpoint, accessKey, secretKey, useSSL.
func ConfigFromSecretData(data map[string][]byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, fmt.Errorf("secret data is empty")
	}
	endpoint := firstSecretString(data, "MINIO_ENDPOINT", "endpoint")
	access := firstSecretString(data, "MINIO_ACCESS_KEY", "accessKey", "accessKeyID")
	secret := firstSecretString(data, "MINIO_SECRET_KEY", "secretKey", "secretAccessKey")
	useSSLStr := firstSecretString(data, "MINIO_USE_SSL", "useSSL")
	useSSL := useSSLStr == "true" || useSSLStr == "1" || strings.EqualFold(useSSLStr, "yes")
	if endpoint == "" || access == "" || secret == "" {
		return Config{}, fmt.Errorf("secret must define endpoint (MINIO_ENDPOINT or endpoint), access key, and secret key")
	}
	return Config{
		Endpoint:  endpoint,
		AccessKey: access,
		SecretKey: secret,
		UseSSL:    useSSL,
	}, nil
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
func (c *Client) CreateBucket(ctx context.Context, bucketName, region string, objectLocking bool) error {
	exists, err := c.s3.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("checking bucket existence: %w", err)
	}
	if exists {
		klog.V(4).Infof("Bucket %q already exists, skipping creation", bucketName)
		return nil
	}

	opts := miniogo.MakeBucketOptions{
		Region:        region,
		ObjectLocking: objectLocking,
	}
	if err := c.s3.MakeBucket(ctx, bucketName, opts); err != nil {
		return fmt.Errorf("creating bucket %q: %w", bucketName, err)
	}

	klog.Infof("Created bucket %q in region %q (objectLock=%v)", bucketName, region, objectLocking)
	return nil
}

// SetBucketQuota sets a hard quota on the bucket.
func (c *Client) SetBucketQuota(ctx context.Context, bucketName string, quotaBytes int64) error {
	if quotaBytes <= 0 {
		return nil
	}
	return c.admin.SetBucketQuota(ctx, bucketName, &madmin.BucketQuota{
		Quota: uint64(quotaBytes),
		Type:  madmin.HardQuota,
	})
}

// SetBucketLifecycle configures lifecycle rules on a bucket.
func (c *Client) SetBucketLifecycle(ctx context.Context, bucketName string, cfg *lifecycle.Configuration) error {
	return c.s3.SetBucketLifecycle(ctx, bucketName, cfg)
}

// SetBucketReplication configures replication on a bucket.
func (c *Client) SetBucketReplication(ctx context.Context, bucketName string, cfg replication.Config) error {
	return c.s3.SetBucketReplication(ctx, bucketName, cfg)
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

// GrantAccess creates or updates a MinIO user scoped to the given bucket and returns credentials.
// The returned map contains: accessKeyID, accessSecretKey, bucketName, endpoint.
func (c *Client) GrantAccess(
	ctx context.Context,
	bucketName, accountID, accessType, existingAccessKey, existingSecretKey string,
) (map[string]string, error) {
	accessKey := existingAccessKey
	secretKey := existingSecretKey
	if accessKey == "" {
		accessKey = accessKeyForAccount(accountID)
	}
	if secretKey == "" {
		secretKey = secretForAccount(accountID)
	}

	// Create or update the user credentials
	if err := c.admin.AddUser(ctx, accessKey, secretKey); err != nil {
		return nil, fmt.Errorf("creating MinIO user %q: %w", accessKey, err)
	}

	// Create a bucket-scoped policy
	policyName := fmt.Sprintf("cosi-%s-%s", bucketName, accountID)
	policy := bucketPolicy(bucketName)
	if strings.EqualFold(accessType, "ReadOnly") {
		policy = bucketPolicyReadOnly(bucketName)
	}

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
func (c *Client) RevokeAccess(ctx context.Context, bucketName, accountID, accessKey string) error {
	policyName := fmt.Sprintf("cosi-%s-%s", bucketName, accountID)

	if accessKey == "" {
		accessKey = accessKeyForAccount(accountID)
	}
	if err := c.admin.RemoveUser(ctx, accessKey); err != nil {
		klog.Warningf("Could not remove user %q: %v (may already be deleted)", accessKey, err)
	}

	// Remove policy
	if err := c.admin.RemoveCannedPolicy(ctx, policyName); err != nil {
		klog.Warningf("Could not remove policy %q: %v (may already be deleted)", policyName, err)
	}

	klog.Infof("Revoked access to bucket %q for account %q", bucketName, accountID)
	return nil
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func accessKeyForAccount(accountID string) string {
	normalized := strings.ToLower(nonAlnum.ReplaceAllString(accountID, ""))
	if len(normalized) > 12 {
		normalized = normalized[:12]
	}
	if normalized == "" {
		normalized = "user"
	}
	return "cosi" + normalized
}

func secretForAccount(accountID string) string {
	sum := sha1.Sum([]byte(accountID))
	return fmt.Sprintf("%x%x", sum[:], sum[:4])
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

// bucketPolicyReadOnly returns an IAM policy document scoped to a single bucket.
func bucketPolicyReadOnly(bucketName string) map[string]interface{} {
	return map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"s3:GetObject",
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
