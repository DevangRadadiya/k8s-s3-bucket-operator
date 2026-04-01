package aws

import (
	"encoding/json"
	"fmt"
)

// bucketPolicyDocument returns a least-privilege IAM policy document scoped to one bucket.
// AccessType semantics match our existing BucketClaim behavior ("ReadOnly" vs everything else).
func bucketPolicyDocument(bucketName string, readOnly bool) ([]byte, error) {
	actions := []string{
		"s3:GetObject",
		"s3:ListBucket",
		"s3:GetBucketLocation",
	}
	if !readOnly {
		actions = append(actions, "s3:PutObject", "s3:DeleteObject")
	}

	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Effect":   "Allow",
				"Action":   actions,
				"Resource": []string{fmt.Sprintf("arn:aws:s3:::%s", bucketName), fmt.Sprintf("arn:aws:s3:::%s/*", bucketName)},
			},
		},
	}
	return json.Marshal(doc)
}

