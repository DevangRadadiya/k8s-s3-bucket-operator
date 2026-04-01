package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/minio/minio-go/v7/pkg/replication"
)

// s3API is the subset of the AWS S3 client we use.
type s3API interface {
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error)
	PutBucketLifecycleConfiguration(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error)
	PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error)
	PutBucketOwnershipControls(ctx context.Context, params *s3.PutBucketOwnershipControlsInput, optFns ...func(*s3.Options)) (*s3.PutBucketOwnershipControlsOutput, error)
	PutBucketEncryption(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error)
	PutBucketPolicy(ctx context.Context, params *s3.PutBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error)
}

// iamAPI is the subset of the AWS IAM client we use.
type iamAPI interface {
	CreateUser(ctx context.Context, params *iam.CreateUserInput, optFns ...func(*iam.Options)) (*iam.CreateUserOutput, error)
	DeleteUser(ctx context.Context, params *iam.DeleteUserInput, optFns ...func(*iam.Options)) (*iam.DeleteUserOutput, error)
	CreateAccessKey(ctx context.Context, params *iam.CreateAccessKeyInput, optFns ...func(*iam.Options)) (*iam.CreateAccessKeyOutput, error)
	DeleteAccessKey(ctx context.Context, params *iam.DeleteAccessKeyInput, optFns ...func(*iam.Options)) (*iam.DeleteAccessKeyOutput, error)
	ListAccessKeys(ctx context.Context, params *iam.ListAccessKeysInput, optFns ...func(*iam.Options)) (*iam.ListAccessKeysOutput, error)
	PutUserPolicy(ctx context.Context, params *iam.PutUserPolicyInput, optFns ...func(*iam.Options)) (*iam.PutUserPolicyOutput, error)
	DeleteUserPolicy(ctx context.Context, params *iam.DeleteUserPolicyInput, optFns ...func(*iam.Options)) (*iam.DeleteUserPolicyOutput, error)
}

// Provider implements the backend.Provider interface using AWS S3 + IAM.
type Provider struct {
	s3       s3API
	iam      iamAPI
	region   string
	endpoint string

	// bucketPolicyJSON, when set, is an additional bucket policy document applied by ConfigureBucketSecurity.
	// It is typically loaded from BucketClass.bucketPolicyRef by the resolver.
	bucketPolicyJSON []byte
}

// New creates a real AWS provider from config.
func New(ctx context.Context, cfg Config) (*Provider, error) {
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if strings.TrimSpace(cfg.S3Endpoint) != "" {
			o.BaseEndpoint = aws.String(strings.TrimSpace(cfg.S3Endpoint))
			o.UsePathStyle = true
		}
	})
	iamClient := iam.NewFromConfig(awsCfg, func(o *iam.Options) {
		if strings.TrimSpace(cfg.IAMEndpoint) != "" {
			o.BaseEndpoint = aws.String(strings.TrimSpace(cfg.IAMEndpoint))
		}
	})

	ep := strings.TrimSpace(cfg.S3Endpoint)
	if ep == "" {
		// Standard AWS S3 endpoint (virtual-hosted style). Returned for client config.
		ep = fmt.Sprintf("https://s3.%s.amazonaws.com", strings.TrimSpace(cfg.Region))
	}

	return &Provider{s3: s3Client, iam: iamClient, region: strings.TrimSpace(cfg.Region), endpoint: ep}, nil
}

// NewWithClients is for unit tests (inject fakes).
func NewWithClients(region, endpoint string, s3c s3API, iamc iamAPI) *Provider {
	region = strings.TrimSpace(region)
	if endpoint == "" && region != "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", region)
	}
	return &Provider{s3: s3c, iam: iamc, region: region, endpoint: strings.TrimSpace(endpoint)}
}

func (p *Provider) WithBucketPolicyDocument(raw []byte) *Provider {
	if p == nil {
		return p
	}
	if len(raw) == 0 {
		p.bucketPolicyJSON = nil
		return p
	}
	p.bucketPolicyJSON = append([]byte(nil), raw...)
	return p
}

func paramBool(parameters map[string]string, key string, def bool) (bool, error) {
	if parameters == nil {
		return def, nil
	}
	raw, ok := parameters[key]
	if !ok {
		return def, nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return def, fmt.Errorf("invalid bool for %q: %q", key, raw)
	}
	return v, nil
}

func paramString(parameters map[string]string, key, def string) string {
	if parameters == nil {
		return def
	}
	raw := strings.TrimSpace(parameters[key])
	if raw == "" {
		return def
	}
	return raw
}

func tlsOnlyBucketPolicy(bucketName string) map[string]any {
	return map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Sid":    "DenyInsecureTransport",
				"Effect": "Deny",
				"Principal": map[string]any{
					"AWS": "*",
				},
				"Action": []string{"s3:*"},
				"Resource": []string{
					fmt.Sprintf("arn:aws:s3:::%s", bucketName),
					fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
				},
				"Condition": map[string]any{
					"Bool": map[string]any{
						"aws:SecureTransport": "false",
					},
				},
			},
		},
	}
}

func mergePolicyDocuments(base map[string]any, userRaw []byte) (map[string]any, error) {
	if len(userRaw) == 0 {
		return base, nil
	}
	var user map[string]any
	if err := json.Unmarshal(userRaw, &user); err != nil {
		return nil, fmt.Errorf("invalid bucket policy JSON: %w", err)
	}

	getStatements := func(doc map[string]any) ([]any, error) {
		st, ok := doc["Statement"]
		if !ok {
			return nil, nil
		}
		switch v := st.(type) {
		case []any:
			return v, nil
		case []map[string]any:
			out := make([]any, 0, len(v))
			for i := range v {
				out = append(out, v[i])
			}
			return out, nil
		default:
			return nil, fmt.Errorf("bucket policy Statement must be an array")
		}
	}

	baseSt, err := getStatements(base)
	if err != nil {
		return nil, err
	}
	userSt, err := getStatements(user)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(user)+len(base))
	for k, v := range user {
		out[k] = v
	}
	// Ensure Version exists (prefer user version if present).
	if _, ok := out["Version"]; !ok {
		if v, ok := base["Version"]; ok {
			out["Version"] = v
		} else {
			out["Version"] = "2012-10-17"
		}
	}
	// Merge statements: base (guardrails) first, then user statements.
	if len(baseSt) > 0 || len(userSt) > 0 {
		merged := make([]any, 0, len(baseSt)+len(userSt))
		merged = append(merged, baseSt...)
		merged = append(merged, userSt...)
		out["Statement"] = merged
	}
	return out, nil
}

// ConfigureBucketSecurity applies security hardening to an existing bucket.
// Defaults are secure when parameters are missing.
func (p *Provider) ConfigureBucketSecurity(ctx context.Context, bucketName string, parameters map[string]string) error {
	if p == nil || p.s3 == nil {
		return fmt.Errorf("aws provider not configured (s3 client nil)")
	}

	blockPublicAccess, err := paramBool(parameters, "security.blockPublicAccess", true)
	if err != nil {
		return err
	}
	disableAcls, err := paramBool(parameters, "security.disableAcls", true)
	if err != nil {
		return err
	}
	tlsOnlyPolicy, err := paramBool(parameters, "security.tlsOnlyPolicy", true)
	if err != nil {
		return err
	}
	defaultEnc := strings.ToUpper(paramString(parameters, "security.defaultEncryption", "SSE-S3"))
	kmsKeyArn := paramString(parameters, "security.kmsKeyArn", "")

	if blockPublicAccess {
		_, err := p.s3.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
			Bucket: aws.String(bucketName),
			PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
				BlockPublicAcls:       aws.Bool(true),
				IgnorePublicAcls:      aws.Bool(true),
				BlockPublicPolicy:     aws.Bool(true),
				RestrictPublicBuckets: aws.Bool(true),
			},
		})
		if err != nil {
			return err
		}
	}

	if disableAcls {
		_, err := p.s3.PutBucketOwnershipControls(ctx, &s3.PutBucketOwnershipControlsInput{
			Bucket: aws.String(bucketName),
			OwnershipControls: &s3types.OwnershipControls{
				Rules: []s3types.OwnershipControlsRule{
					{ObjectOwnership: s3types.ObjectOwnershipBucketOwnerEnforced},
				},
			},
		})
		if err != nil {
			return err
		}
	}

	switch defaultEnc {
	case "NONE":
		// no-op
	case "SSE-S3":
		_, err := p.s3.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{
			Bucket: aws.String(bucketName),
			ServerSideEncryptionConfiguration: &s3types.ServerSideEncryptionConfiguration{
				Rules: []s3types.ServerSideEncryptionRule{
					{
						ApplyServerSideEncryptionByDefault: &s3types.ServerSideEncryptionByDefault{
							SSEAlgorithm: s3types.ServerSideEncryptionAes256,
						},
					},
				},
			},
		})
		if err != nil {
			return err
		}
	case "SSE-KMS":
		if strings.TrimSpace(kmsKeyArn) == "" {
			return fmt.Errorf("security.kmsKeyArn is required when security.defaultEncryption=SSE-KMS")
		}
		_, err := p.s3.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{
			Bucket: aws.String(bucketName),
			ServerSideEncryptionConfiguration: &s3types.ServerSideEncryptionConfiguration{
				Rules: []s3types.ServerSideEncryptionRule{
					{
						ApplyServerSideEncryptionByDefault: &s3types.ServerSideEncryptionByDefault{
							SSEAlgorithm:   s3types.ServerSideEncryptionAwsKms,
							KMSMasterKeyID: aws.String(kmsKeyArn),
						},
					},
				},
			},
		})
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported security.defaultEncryption %q (expected SSE-S3, SSE-KMS, none)", defaultEnc)
	}

	// Bucket policy (guardrails).
	if !tlsOnlyPolicy && len(p.bucketPolicyJSON) == 0 {
		return nil
	}
	var base map[string]any
	if tlsOnlyPolicy {
		base = tlsOnlyBucketPolicy(bucketName)
	} else {
		base = map[string]any{"Version": "2012-10-17", "Statement": []any{}}
	}
	merged, err := mergePolicyDocuments(base, p.bucketPolicyJSON)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	_, err = p.s3.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucketName),
		Policy: aws.String(string(raw)),
	})
	return err
}

func (p *Provider) CreateBucket(ctx context.Context, bucketName, region string, objectLocking bool) error {
	region = strings.TrimSpace(region)

	in := &s3.CreateBucketInput{Bucket: aws.String(bucketName)}
	if objectLocking {
		in.ObjectLockEnabledForBucket = aws.Bool(true)
	}
	// For region-specific endpoints, AWS requires the LocationConstraint for non-us-east-1.
	if region != "" && !strings.EqualFold(region, "us-east-1") {
		in.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(region),
		}
	}
	_, err := p.s3.CreateBucket(ctx, in)
	if err == nil {
		return nil
	}

	// Treat "already exists/owned" as idempotent.
	var owned *s3types.BucketAlreadyOwnedByYou
	if errors.As(err, &owned) {
		return nil
	}
	var exists *s3types.BucketAlreadyExists
	if errors.As(err, &exists) {
		return nil
	}
	return err
}

func (p *Provider) SetBucketQuota(ctx context.Context, bucketName string, quotaBytes int64) error {
	_ = ctx
	_ = bucketName
	_ = quotaBytes
	// AWS S3 doesn't support per-bucket hard quotas via API; treat as no-op.
	return nil
}

func (p *Provider) SetBucketLifecycle(ctx context.Context, bucketName string, cfg *lifecycle.Configuration) error {
	if cfg == nil || len(cfg.Rules) == 0 {
		return nil
	}

	var rules []s3types.LifecycleRule
	for _, r := range cfg.Rules {
		rule := s3types.LifecycleRule{
			Status: s3types.ExpirationStatusEnabled,
		}
		if strings.TrimSpace(r.ID) != "" {
			rule.ID = aws.String(r.ID)
		}
		if strings.TrimSpace(r.RuleFilter.Prefix) != "" {
			rule.Filter = &s3types.LifecycleRuleFilter{Prefix: aws.String(r.RuleFilter.Prefix)}
		}
		if r.Expiration.Days > 0 {
			rule.Expiration = &s3types.LifecycleExpiration{Days: aws.Int32(int32(r.Expiration.Days))}
		}
		rules = append(rules, rule)
	}

	_, err := p.s3.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(bucketName),
		LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{
			Rules: rules,
		},
	})
	return err
}

func (p *Provider) SetBucketReplication(ctx context.Context, bucketName string, cfg replication.Config) error {
	_ = ctx
	_ = bucketName
	_ = cfg
	// Not implemented in Wave 3 MVP.
	return nil
}

func userNameForAccount(accountID string) string {
	normalized := strings.ToLower(accountID)
	normalized = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, normalized)
	if len(normalized) > 64 {
		normalized = normalized[:64]
	}
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		normalized = "user"
	}
	return "cosi-" + normalized
}

func policyName(bucketName, accountID string) string {
	return fmt.Sprintf("bucket-%s-%s", bucketName, userNameForAccount(accountID))
}

func (p *Provider) GrantAccess(ctx context.Context, bucketName, accountID, accessType, existingAccessKey, existingSecretKey string) (map[string]string, error) {
	if p == nil || p.iam == nil {
		return nil, fmt.Errorf("aws provider not configured (iam client nil)")
	}

	userName := userNameForAccount(accountID)

	// Ensure user exists (idempotent).
	_, err := p.iam.CreateUser(ctx, &iam.CreateUserInput{UserName: aws.String(userName)})
	if err != nil {
		var ent *iamtypes.EntityAlreadyExistsException
		if !errors.As(err, &ent) {
			return nil, err
		}
	}

	readOnly := strings.EqualFold(strings.TrimSpace(accessType), "ReadOnly")
	pol, err := bucketPolicyDocument(bucketName, readOnly)
	if err != nil {
		return nil, err
	}

	pn := policyName(bucketName, accountID)
	if _, err := p.iam.PutUserPolicy(ctx, &iam.PutUserPolicyInput{
		UserName:       aws.String(userName),
		PolicyName:     aws.String(pn),
		PolicyDocument: aws.String(string(pol)),
	}); err != nil {
		return nil, err
	}

	// If existing keys are supplied, keep them (so reconciling doesn't rotate).
	if strings.TrimSpace(existingAccessKey) != "" && strings.TrimSpace(existingSecretKey) != "" {
		return map[string]string{
			"accessKeyID":     strings.TrimSpace(existingAccessKey),
			"accessSecretKey": strings.TrimSpace(existingSecretKey),
			"bucketName":      bucketName,
			"endpoint":        "",
		}, nil
	}

	out, err := p.iam.CreateAccessKey(ctx, &iam.CreateAccessKeyInput{UserName: aws.String(userName)})
	if err != nil {
		// Be idempotent: if the user already has 2 keys (AWS quota), delete old keys and retry.
		// The AWS SDK may wrap this as a generic operation error, so match on the message too.
		var limit *iamtypes.LimitExceededException
		if (errors.As(err, &limit) && strings.Contains(limit.Error(), "AccessKeysPerUser")) ||
			strings.Contains(err.Error(), "AccessKeysPerUser") {
			if lk, lerr := p.iam.ListAccessKeys(ctx, &iam.ListAccessKeysInput{UserName: aws.String(userName)}); lerr == nil {
				for _, meta := range lk.AccessKeyMetadata {
					_, _ = p.iam.DeleteAccessKey(ctx, &iam.DeleteAccessKeyInput{
						UserName:    aws.String(userName),
						AccessKeyId: meta.AccessKeyId,
					})
				}
			}
			out, err = p.iam.CreateAccessKey(ctx, &iam.CreateAccessKeyInput{UserName: aws.String(userName)})
		}
		if err != nil {
			return nil, err
		}
	}

	return map[string]string{
		"accessKeyID":     aws.ToString(out.AccessKey.AccessKeyId),
		"accessSecretKey": aws.ToString(out.AccessKey.SecretAccessKey),
		"bucketName":      bucketName,
		"endpoint":        strings.TrimSpace(p.endpoint),
	}, nil
}

func (p *Provider) RevokeAccess(ctx context.Context, bucketName, accountID, accessKey string) error {
	if p == nil || p.iam == nil {
		return fmt.Errorf("aws provider not configured (iam client nil)")
	}

	userName := userNameForAccount(accountID)
	pn := policyName(bucketName, accountID)

	// Best-effort cleanup; IAM returns NoSuchEntity for already-deleted resources.
	_, _ = p.iam.DeleteUserPolicy(ctx, &iam.DeleteUserPolicyInput{UserName: aws.String(userName), PolicyName: aws.String(pn)})

	if strings.TrimSpace(accessKey) != "" {
		_, _ = p.iam.DeleteAccessKey(ctx, &iam.DeleteAccessKeyInput{
			UserName:    aws.String(userName),
			AccessKeyId: aws.String(strings.TrimSpace(accessKey)),
		})
	} else {
		// Best-effort: delete any keys still attached to the user.
		if lk, err := p.iam.ListAccessKeys(ctx, &iam.ListAccessKeysInput{UserName: aws.String(userName)}); err == nil {
			for _, meta := range lk.AccessKeyMetadata {
				_, _ = p.iam.DeleteAccessKey(ctx, &iam.DeleteAccessKeyInput{
					UserName:    aws.String(userName),
					AccessKeyId: meta.AccessKeyId,
				})
			}
		}
	}

	_, _ = p.iam.DeleteUser(ctx, &iam.DeleteUserInput{UserName: aws.String(userName)})
	return nil
}

func (p *Provider) DeleteBucket(ctx context.Context, bucketName string) error {
	_, err := p.s3.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucketName)})
	if err == nil {
		return nil
	}
	// idempotent delete
	var nse *s3types.NoSuchBucket
	if errors.As(err, &nse) {
		return nil
	}
	return err
}
