package backend

import (
	"testing"

	v1alpha1 "github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
)

func TestKindForBucketClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		class   *v1alpha1.BucketClass
		want    Kind
		wantErr bool
	}{
		{"nil", nil, KindMinIO, false},
		{"empty", &v1alpha1.BucketClass{}, KindMinIO, false},
		{"MinIO", &v1alpha1.BucketClass{Backend: "MinIO"}, KindMinIO, false},
		{"minio lower", &v1alpha1.BucketClass{Backend: "minio"}, KindMinIO, false},
		{"AWS", &v1alpha1.BucketClass{Backend: "AWS"}, KindAWS, false},
		{"aws lower", &v1alpha1.BucketClass{Backend: "aws"}, KindAWS, false},
		{"bad", &v1alpha1.BucketClass{Backend: "gcs"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := KindForBucketClass(tt.class)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("kind=%q want %q", got, tt.want)
			}
		})
	}
}
