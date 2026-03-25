package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1alpha1 "github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type tempNetErr struct{}

func (tempNetErr) Error() string   { return "i/o timeout" }
func (tempNetErr) Timeout() bool   { return true }
func (tempNetErr) Temporary() bool { return true }

func TestTrimConditionMessage_Truncates(t *testing.T) {
	// maxConditionMessageRunes is in runes, not bytes.
	longMsg := strings.Repeat("a", maxConditionMessageRunes+10)
	out := trimConditionMessage(longMsg)

	if out == longMsg {
		t.Fatalf("expected truncation, got full message")
	}
	runeCount := []rune(out)
	if runeCount[len(runeCount)-1] != '…' {
		t.Fatalf("expected ellipsis at end, got %q", out)
	}
	if len(runeCount) != maxConditionMessageRunes+1 {
		t.Fatalf("expected %d runes, got %d", maxConditionMessageRunes+1, len(runeCount))
	}
}

func TestIsTransientMinioError(t *testing.T) {
	if !isTransientMinioError(tempNetErr{}) {
		t.Fatalf("expected net.Error timeout to be treated as transient")
	}
	if !isTransientMinioError(errors.New("connection refused")) {
		t.Fatalf("expected substring-based transient detection to return true")
	}
	if isTransientMinioError(errors.New("permission denied")) {
		t.Fatalf("expected non-transient error to return false")
	}
}

func TestMergeClaimStatus_NoOpWhenReadyBoundAndSameGeneration(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = addToScheme(scheme)

	claim := &v1alpha1.BucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "c1",
			Namespace:  "ns1",
			Generation: 3,
		},
		Status: v1alpha1.BucketClaimStatus{
			Phase: "Bound",
			Conditions: []metav1.Condition{
				{
					Type:               claimConditionReady,
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 3,
				},
			},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.BucketClaim{}).
		WithObjects(claim).
		Build()

	r := &BucketClaimReconciler{Client: cl, Scheme: scheme}
	ctx := context.Background()

	err := r.mergeClaimStatus(ctx, types.NamespacedName{Name: "c1", Namespace: "ns1"}, func(c *v1alpha1.BucketClaim) {
		c.Status.Phase = "Pending"
	})
	if err != nil {
		t.Fatalf("mergeClaimStatus returned error: %v", err)
	}

	updated := &v1alpha1.BucketClaim{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "c1", Namespace: "ns1"}, updated); err != nil {
		t.Fatalf("get updated claim: %v", err)
	}
	if updated.Status.Phase != "Bound" {
		t.Fatalf("expected phase Bound, got %q", updated.Status.Phase)
	}
}

func TestMergeClaimStatus_UpdatesWhenNotReadyBound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = addToScheme(scheme)

	claim := &v1alpha1.BucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "c1",
			Namespace:  "ns1",
			Generation: 1,
		},
		Status: v1alpha1.BucketClaimStatus{
			Phase: "Pending",
			Conditions: []metav1.Condition{
				{
					Type:               claimConditionReady,
					Status:             metav1.ConditionFalse,
					ObservedGeneration: 1,
				},
			},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.BucketClaim{}).
		WithObjects(claim).
		Build()

	r := &BucketClaimReconciler{Client: cl, Scheme: scheme}
	ctx := context.Background()

	err := r.mergeClaimStatus(ctx, types.NamespacedName{Name: "c1", Namespace: "ns1"}, func(c *v1alpha1.BucketClaim) {
		c.Status.Phase = "Bound"
		meta.SetStatusCondition(&c.Status.Conditions, metav1.Condition{
			Type:               claimConditionReady,
			Status:             metav1.ConditionTrue,
			Reason:             "x",
			Message:            "y",
			ObservedGeneration: c.GetGeneration(),
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
	})
	if err != nil {
		t.Fatalf("mergeClaimStatus returned error: %v", err)
	}

	updated := &v1alpha1.BucketClaim{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "c1", Namespace: "ns1"}, updated); err != nil {
		t.Fatalf("get updated claim: %v", err)
	}
	if updated.Status.Phase != "Bound" {
		t.Fatalf("expected phase Bound, got %q", updated.Status.Phase)
	}
	if !meta.IsStatusConditionTrue(updated.Status.Conditions, claimConditionReady) {
		t.Fatalf("expected Ready condition to be True")
	}
}

// addToScheme exists to keep tests in this package from having to import v1alpha1 everywhere.
func addToScheme(scheme *runtime.Scheme) error {
	// The actual AddToScheme is in api/v1alpha1. This helper keeps the test file tidy.
	return v1alpha1.AddToScheme(scheme)
}
