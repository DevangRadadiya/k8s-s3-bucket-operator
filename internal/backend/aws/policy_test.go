package aws

import (
	"encoding/json"
	"testing"
)

func TestBucketPolicyDocument_ReadOnly(t *testing.T) {
	t.Parallel()

	raw, err := bucketPolicyDocument("my-bucket", true)
	if err != nil {
		t.Fatalf("bucketPolicyDocument: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stmt := doc["Statement"].([]any)[0].(map[string]any)
	actions := stmt["Action"].([]any)

	for _, a := range actions {
		if a.(string) == "s3:PutObject" || a.(string) == "s3:DeleteObject" {
			t.Fatalf("expected no write actions, got %v", actions)
		}
	}
}

func TestBucketPolicyDocument_ReadWrite(t *testing.T) {
	t.Parallel()

	raw, err := bucketPolicyDocument("my-bucket", false)
	if err != nil {
		t.Fatalf("bucketPolicyDocument: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stmt := doc["Statement"].([]any)[0].(map[string]any)
	actions := stmt["Action"].([]any)

	var hasPut, hasDel bool
	for _, a := range actions {
		switch a.(string) {
		case "s3:PutObject":
			hasPut = true
		case "s3:DeleteObject":
			hasDel = true
		}
	}
	if !hasPut || !hasDel {
		t.Fatalf("expected put+delete, got %v", actions)
	}
}

