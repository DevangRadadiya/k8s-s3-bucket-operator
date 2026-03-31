package cosi

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Central place for objectstorage.k8s.io API identifiers.
//
// Today we speak v1alpha1. If/when upstream COSI moves to GA (e.g. v1),
// we should be able to update these constants and adapt call sites in one place.

const (
	ObjectStorageAPIGroup   = "objectstorage.k8s.io"
	ObjectStorageAPIVersion = "v1alpha1"
)

const (
	KindBucket       = "Bucket"
	KindBucketAccess = "BucketAccess"
)

func BucketGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   ObjectStorageAPIGroup,
		Version: ObjectStorageAPIVersion,
		Kind:    KindBucket,
	}
}

func BucketAccessGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   ObjectStorageAPIGroup,
		Version: ObjectStorageAPIVersion,
		Kind:    KindBucketAccess,
	}
}

