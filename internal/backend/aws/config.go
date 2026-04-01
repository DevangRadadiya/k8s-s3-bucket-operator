package aws

import (
	"fmt"
	"strings"
)

// Config holds the AWS settings needed for S3 + IAM operations.
type Config struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	// S3Endpoint optionally overrides the S3 endpoint (useful for S3-compatible backends / tests).
	S3Endpoint string
}

func firstSecretString(data map[string][]byte, keys ...string) string {
	for _, k := range keys {
		if v, ok := data[k]; ok && len(v) > 0 {
			return strings.TrimSpace(string(v))
		}
	}
	return ""
}

// ConfigFromSecretData builds Config from Secret .Data.
func ConfigFromSecretData(data map[string][]byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, fmt.Errorf("secret data is empty")
	}
	region := firstSecretString(data, "AWS_REGION", "region")
	accessKeyID := firstSecretString(data, "AWS_ACCESS_KEY_ID", "accessKeyID")
	secretAccessKey := firstSecretString(data, "AWS_SECRET_ACCESS_KEY", "secretAccessKey", "secretAccessKeyID")
	s3Endpoint := firstSecretString(data, "AWS_S3_ENDPOINT", "s3Endpoint")

	if region == "" || accessKeyID == "" || secretAccessKey == "" {
		return Config{}, fmt.Errorf("secret must define AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY")
	}
	return Config{
		Region:          region,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		S3Endpoint:      s3Endpoint,
	}, nil
}

