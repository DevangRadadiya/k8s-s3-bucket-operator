package cosi

import (
	"strings"
	"testing"
)

func TestBucketClassNameFromBucketObjectName(t *testing.T) {
	got := BucketClassNameFromBucketObjectName("minio-standard01234567-0123-0123-0123-012345abcdef")
	if got != "minio-standard" {
		t.Fatalf("expected minio-standard, got %q", got)
	}
	if BucketClassNameFromBucketObjectName("nope") != "" {
		t.Fatalf("expected empty for non-matching name")
	}
}

func TestSyntheticClaimName_Truncates(t *testing.T) {
	in := strings.Repeat("a", 100)
	got := SyntheticClaimName(in)
	if len(got) != 63 {
		t.Fatalf("expected truncation to 63, got len=%d (%q)", len(got), got)
	}
}
