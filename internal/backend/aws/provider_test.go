package aws

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

type fakeS3 struct {
	createBucketCalls         int
	deleteBucketCalls         int
	putLifecycleCalls         int
	putPublicAccessBlockCalls int
	putOwnershipControlsCalls int
	putEncryptionCalls        int
	putBucketPolicyCalls      int

	lastBucket string
}

func (f *fakeS3) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	_ = ctx
	f.createBucketCalls++
	f.lastBucket = aws.ToString(params.Bucket)
	return &s3.CreateBucketOutput{}, nil
}

func (f *fakeS3) DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	_ = ctx
	f.deleteBucketCalls++
	f.lastBucket = aws.ToString(params.Bucket)
	return &s3.DeleteBucketOutput{}, nil
}

func (f *fakeS3) PutBucketLifecycleConfiguration(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
	_ = ctx
	f.putLifecycleCalls++
	f.lastBucket = aws.ToString(params.Bucket)
	return &s3.PutBucketLifecycleConfigurationOutput{}, nil
}

func (f *fakeS3) PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	_ = ctx
	f.putPublicAccessBlockCalls++
	f.lastBucket = aws.ToString(params.Bucket)
	return &s3.PutPublicAccessBlockOutput{}, nil
}

func (f *fakeS3) PutBucketOwnershipControls(ctx context.Context, params *s3.PutBucketOwnershipControlsInput, optFns ...func(*s3.Options)) (*s3.PutBucketOwnershipControlsOutput, error) {
	_ = ctx
	f.putOwnershipControlsCalls++
	f.lastBucket = aws.ToString(params.Bucket)
	return &s3.PutBucketOwnershipControlsOutput{}, nil
}

func (f *fakeS3) PutBucketEncryption(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	_ = ctx
	f.putEncryptionCalls++
	f.lastBucket = aws.ToString(params.Bucket)
	return &s3.PutBucketEncryptionOutput{}, nil
}

func (f *fakeS3) PutBucketPolicy(ctx context.Context, params *s3.PutBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error) {
	_ = ctx
	f.putBucketPolicyCalls++
	f.lastBucket = aws.ToString(params.Bucket)
	return &s3.PutBucketPolicyOutput{}, nil
}

type fakeIAM struct {
	createUserCalls      int
	putUserPolicyCalls   int
	createAccessKeyCalls int
	deletePolicyCalls    int
	deleteAccessKeyCalls int
	deleteUserCalls      int
	listAccessKeysCalls  int

	lastUserName   string
	lastPolicyName string
	lastPolicyDoc  string

	createAccessKeyFn func(ctx context.Context, params *iam.CreateAccessKeyInput, optFns ...func(*iam.Options)) (*iam.CreateAccessKeyOutput, error)
}

func (f *fakeIAM) CreateUser(ctx context.Context, params *iam.CreateUserInput, optFns ...func(*iam.Options)) (*iam.CreateUserOutput, error) {
	_ = ctx
	f.createUserCalls++
	f.lastUserName = aws.ToString(params.UserName)
	return nil, &iamtypes.EntityAlreadyExistsException{}
}

func (f *fakeIAM) DeleteUser(ctx context.Context, params *iam.DeleteUserInput, optFns ...func(*iam.Options)) (*iam.DeleteUserOutput, error) {
	_ = ctx
	f.deleteUserCalls++
	f.lastUserName = aws.ToString(params.UserName)
	return &iam.DeleteUserOutput{}, nil
}

func (f *fakeIAM) CreateAccessKey(ctx context.Context, params *iam.CreateAccessKeyInput, optFns ...func(*iam.Options)) (*iam.CreateAccessKeyOutput, error) {
	_ = ctx
	f.createAccessKeyCalls++
	f.lastUserName = aws.ToString(params.UserName)
	if f.createAccessKeyFn != nil {
		return f.createAccessKeyFn(ctx, params, optFns...)
	}
	return &iam.CreateAccessKeyOutput{
		AccessKey: &iamtypes.AccessKey{
			AccessKeyId:     aws.String("AKIA_TEST"),
			SecretAccessKey: aws.String("SECRET_TEST"),
		},
	}, nil
}

func (f *fakeIAM) DeleteAccessKey(ctx context.Context, params *iam.DeleteAccessKeyInput, optFns ...func(*iam.Options)) (*iam.DeleteAccessKeyOutput, error) {
	_ = ctx
	f.deleteAccessKeyCalls++
	return &iam.DeleteAccessKeyOutput{}, nil
}

func (f *fakeIAM) ListAccessKeys(ctx context.Context, params *iam.ListAccessKeysInput, optFns ...func(*iam.Options)) (*iam.ListAccessKeysOutput, error) {
	_ = ctx
	_ = params
	f.listAccessKeysCalls++
	return &iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamtypes.AccessKeyMetadata{
			{AccessKeyId: aws.String("AKIA_OLD_1")},
			{AccessKeyId: aws.String("AKIA_OLD_2")},
		},
	}, nil
}

func (f *fakeIAM) PutUserPolicy(ctx context.Context, params *iam.PutUserPolicyInput, optFns ...func(*iam.Options)) (*iam.PutUserPolicyOutput, error) {
	_ = ctx
	f.putUserPolicyCalls++
	f.lastUserName = aws.ToString(params.UserName)
	f.lastPolicyName = aws.ToString(params.PolicyName)
	f.lastPolicyDoc = aws.ToString(params.PolicyDocument)
	return &iam.PutUserPolicyOutput{}, nil
}

func (f *fakeIAM) DeleteUserPolicy(ctx context.Context, params *iam.DeleteUserPolicyInput, optFns ...func(*iam.Options)) (*iam.DeleteUserPolicyOutput, error) {
	_ = ctx
	f.deletePolicyCalls++
	f.lastUserName = aws.ToString(params.UserName)
	f.lastPolicyName = aws.ToString(params.PolicyName)
	return &iam.DeleteUserPolicyOutput{}, nil
}

type s3Owned struct{}

var _ s3API = (*s3Owned)(nil)

func (s3Owned) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	_ = ctx
	_ = params
	return nil, &s3types.BucketAlreadyOwnedByYou{}
}

func (s3Owned) DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	_ = ctx
	_ = params
	return &s3.DeleteBucketOutput{}, nil
}

func (s3Owned) PutBucketLifecycleConfiguration(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutBucketLifecycleConfigurationOutput{}, nil
}

func (s3Owned) PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutPublicAccessBlockOutput{}, nil
}

func (s3Owned) PutBucketOwnershipControls(ctx context.Context, params *s3.PutBucketOwnershipControlsInput, optFns ...func(*s3.Options)) (*s3.PutBucketOwnershipControlsOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutBucketOwnershipControlsOutput{}, nil
}

func (s3Owned) PutBucketEncryption(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutBucketEncryptionOutput{}, nil
}

func (s3Owned) PutBucketPolicy(ctx context.Context, params *s3.PutBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutBucketPolicyOutput{}, nil
}

type s3NoSuch struct{}

var _ s3API = (*s3NoSuch)(nil)

func (s3NoSuch) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	_ = ctx
	_ = params
	return &s3.CreateBucketOutput{}, nil
}

func (s3NoSuch) DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	_ = ctx
	_ = params
	return nil, &s3types.NoSuchBucket{}
}

func (s3NoSuch) PutBucketLifecycleConfiguration(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutBucketLifecycleConfigurationOutput{}, nil
}

func (s3NoSuch) PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutPublicAccessBlockOutput{}, nil
}

func (s3NoSuch) PutBucketOwnershipControls(ctx context.Context, params *s3.PutBucketOwnershipControlsInput, optFns ...func(*s3.Options)) (*s3.PutBucketOwnershipControlsOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutBucketOwnershipControlsOutput{}, nil
}

func (s3NoSuch) PutBucketEncryption(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutBucketEncryptionOutput{}, nil
}

func (s3NoSuch) PutBucketPolicy(ctx context.Context, params *s3.PutBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error) {
	_ = ctx
	_ = params
	return &s3.PutBucketPolicyOutput{}, nil
}

func TestProvider_GrantAccess_CreatesPolicyAndAccessKey(t *testing.T) {
	t.Parallel()

	s3f := &fakeS3{}
	iamf := &fakeIAM{}
	p := NewWithClients("eu-west-2", "", s3f, iamf)

	out, err := p.GrantAccess(context.Background(), "b1", "ns1-claim1", "ReadWrite", "", "")
	if err != nil {
		t.Fatalf("GrantAccess: %v", err)
	}
	if out["accessKeyID"] == "" || out["accessSecretKey"] == "" {
		t.Fatalf("expected creds, got %#v", out)
	}
	if iamf.putUserPolicyCalls != 1 || iamf.createAccessKeyCalls != 1 {
		t.Fatalf("expected policy+key, got policy=%d key=%d", iamf.putUserPolicyCalls, iamf.createAccessKeyCalls)
	}
	if !strings.Contains(iamf.lastPolicyDoc, "arn:aws:s3:::b1") {
		t.Fatalf("expected bucket ARN in policy, got %q", iamf.lastPolicyDoc)
	}
	if out["endpoint"] == "" {
		t.Fatalf("expected endpoint, got %#v", out)
	}
}

func TestProvider_GrantAccess_DeletesOldKeysOnLimitExceeded(t *testing.T) {
	t.Parallel()

	s3f := &fakeS3{}
	iamf := &fakeIAM{}

	// Force first CreateAccessKey call to return LimitExceeded.
	first := true
	iamf.createAccessKeyFn = func(ctx context.Context, params *iam.CreateAccessKeyInput, optFns ...func(*iam.Options)) (*iam.CreateAccessKeyOutput, error) {
		if first {
			first = false
			return nil, &iamtypes.LimitExceededException{Message: aws.String("Cannot exceed quota for AccessKeysPerUser: 2")}
		}
		return &iam.CreateAccessKeyOutput{
			AccessKey: &iamtypes.AccessKey{
				AccessKeyId:     aws.String("AKIA_TEST"),
				SecretAccessKey: aws.String("SECRET_TEST"),
			},
		}, nil
	}

	p := NewWithClients("eu-west-2", "", s3f, iamf)
	_, err := p.GrantAccess(context.Background(), "b1", "ns1-claim1", "ReadWrite", "", "")
	if err != nil {
		t.Fatalf("GrantAccess: %v", err)
	}
	if iamf.listAccessKeysCalls == 0 || iamf.deleteAccessKeyCalls == 0 {
		t.Fatalf("expected list+delete keys, got list=%d delete=%d", iamf.listAccessKeysCalls, iamf.deleteAccessKeyCalls)
	}
}

func TestProvider_GrantAccess_UsesExistingKeys_NoRotation(t *testing.T) {
	t.Parallel()

	p := NewWithClients("eu-west-2", "", &fakeS3{}, &fakeIAM{})
	out, err := p.GrantAccess(context.Background(), "b1", "ns1-claim1", "ReadOnly", "EXISTING_AK", "EXISTING_SK")
	if err != nil {
		t.Fatalf("GrantAccess: %v", err)
	}
	if out["accessKeyID"] != "EXISTING_AK" || out["accessSecretKey"] != "EXISTING_SK" {
		t.Fatalf("expected existing keys, got %#v", out)
	}
}

func TestProvider_CreateBucket_IdempotentOnAlreadyOwned(t *testing.T) {
	t.Parallel()

	p := NewWithClients("eu-west-2", "", s3Owned{}, &fakeIAM{})
	if err := p.CreateBucket(context.Background(), "b1", "us-east-1", false); err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
}

func TestProvider_DeleteBucket_IdempotentOnNoSuchBucket(t *testing.T) {
	t.Parallel()

	p := NewWithClients("eu-west-2", "", s3NoSuch{}, &fakeIAM{})
	if err := p.DeleteBucket(context.Background(), "b1"); err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
}

func TestProvider_SetBucketLifecycle_NoOpOnEmpty(t *testing.T) {
	t.Parallel()

	s3f := &fakeS3{}
	p := NewWithClients("eu-west-2", "", s3f, &fakeIAM{})
	if err := p.SetBucketLifecycle(context.Background(), "b1", &lifecycle.Configuration{}); err != nil {
		t.Fatalf("SetBucketLifecycle: %v", err)
	}
	if s3f.putLifecycleCalls != 0 {
		t.Fatalf("expected no PutLifecycle calls, got %d", s3f.putLifecycleCalls)
	}
}

func TestProvider_ConfigureBucketSecurity_Defaults(t *testing.T) {
	t.Parallel()

	s3f := &fakeS3{}
	p := NewWithClients("eu-west-2", "", s3f, &fakeIAM{})

	if err := p.ConfigureBucketSecurity(context.Background(), "b1", nil); err != nil {
		t.Fatalf("ConfigureBucketSecurity: %v", err)
	}
	if s3f.putPublicAccessBlockCalls != 1 {
		t.Fatalf("expected PutPublicAccessBlock 1 call, got %d", s3f.putPublicAccessBlockCalls)
	}
	if s3f.putOwnershipControlsCalls != 1 {
		t.Fatalf("expected PutBucketOwnershipControls 1 call, got %d", s3f.putOwnershipControlsCalls)
	}
	if s3f.putEncryptionCalls != 1 {
		t.Fatalf("expected PutBucketEncryption 1 call, got %d", s3f.putEncryptionCalls)
	}
	if s3f.putBucketPolicyCalls != 1 {
		t.Fatalf("expected PutBucketPolicy 1 call, got %d", s3f.putBucketPolicyCalls)
	}
}

func TestProvider_ConfigureBucketSecurity_MergesUserPolicy(t *testing.T) {
	t.Parallel()

	s3f := &fakeS3{}
	user := []byte(`{"Version":"2012-10-17","Statement":[{"Sid":"Extra","Effect":"Deny","Principal":{"AWS":"*"},"Action":["s3:DeleteBucket"],"Resource":["arn:aws:s3:::b1"]}]}`)
	p := NewWithClients("eu-west-2", "", s3f, &fakeIAM{}).WithBucketPolicyDocument(user)

	if err := p.ConfigureBucketSecurity(context.Background(), "b1", map[string]string{
		"security.tlsOnlyPolicy": "true",
	}); err != nil {
		t.Fatalf("ConfigureBucketSecurity: %v", err)
	}

	// Ensure the merge helper produces valid JSON with Statement array.
	merged, err := mergePolicyDocuments(tlsOnlyBucketPolicy("b1"), user)
	if err != nil {
		t.Fatalf("mergePolicyDocuments: %v", err)
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var chk map[string]any
	if err := json.Unmarshal(raw, &chk); err != nil {
		t.Fatalf("merged policy invalid JSON: %v", err)
	}
}
