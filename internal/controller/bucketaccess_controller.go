package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	v1alpha1 "github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/cosi"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/minio"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const bucketAccessFinalizer = "cosi.objectstorage.k8s.io/bucketaccess-finalizer"

var nonAlnumForAccessKey = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func accessKeyForAccount(accountID string) string {
	normalized := strings.ToLower(nonAlnumForAccessKey.ReplaceAllString(accountID, ""))
	if len(normalized) > 12 {
		normalized = normalized[:12]
	}
	if normalized == "" {
		normalized = "user"
	}
	return "cosi" + normalized
}

type BucketAccessReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Minio  *minio.Client
}

func (r *BucketAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	bucketAccessGVK := cosi.BucketAccessGVK()
	ba := &unstructured.Unstructured{}
	ba.SetGroupVersionKind(bucketAccessGVK)

	if err := r.Get(ctx, req.NamespacedName, ba); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	spec, _ := ba.Object["spec"].(map[string]interface{})
	status, _ := ba.Object["status"].(map[string]interface{})

	bucketClaimName, _ := spec["bucketClaimName"].(string)
	credentialsSecretName, _ := spec["credentialsSecretName"].(string)

	if bucketClaimName == "" || credentialsSecretName == "" {
		// Spec is required by CRD, but handle gracefully.
		logger.Error(fmt.Errorf("missing required spec fields"), "BucketAccess spec incomplete", "bucketClaimName", bucketClaimName, "credentialsSecretName", credentialsSecretName)
		return ctrl.Result{}, nil
	}

	accountID := fmt.Sprintf("%s-%s", req.Namespace, req.Name)

	// Handle deletion: revoke access + delete secret + remove finalizer.
	if !ba.GetDeletionTimestamp().IsZero() {
		if controllerutil.ContainsFinalizer(ba, bucketAccessFinalizer) {
			// Fetch the referenced BucketClaim to learn the backend bucket name.
			claim := &v1alpha1.BucketClaim{}
			if err := r.Get(ctx, types.NamespacedName{Name: bucketClaimName, Namespace: req.Namespace}, claim); err != nil {
				if apierrors.IsNotFound(err) {
					controllerutil.RemoveFinalizer(ba, bucketAccessFinalizer)
					if err := r.Update(ctx, ba); err != nil {
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, err
			}

			bucketName := strings.TrimSpace(claim.Status.BucketName)
			if bucketName == "" {
				bucketName = strings.TrimSpace(claim.Spec.BucketName)
			}
			if bucketName == "" {
				bucketName = fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
			}
			bucketName = strings.ToLower(bucketName)

			accessKeyID := ""
			secret := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Name: credentialsSecretName, Namespace: req.Namespace}, secret); err == nil {
				var payload struct {
					Spec struct {
						S3 struct {
							AccessKeyID string `json:"accessKeyID"`
						} `json:"s3"`
					} `json:"spec"`
				}
				if raw, ok := secret.Data["BucketInfo"]; ok {
					_ = json.Unmarshal(raw, &payload)
					accessKeyID = strings.TrimSpace(payload.Spec.S3.AccessKeyID)
				}
			}
			if accessKeyID == "" {
				// Fallback: compute access key deterministically.
				accessKeyID = accessKeyForAccount(accountID)
			}

			// Revoke access but do not delete the backend bucket.
			if err := r.Minio.RevokeAccess(ctx, bucketName, accountID, accessKeyID); err != nil {
				logger.Error(err, "Failed to revoke access", "bucketName", bucketName, "accountID", accountID)
				// Retry: revocation failure might be transient.
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}

			// Delete credentials secret (best-effort).
			_ = r.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      credentialsSecretName,
					Namespace: req.Namespace,
				},
			})

			controllerutil.RemoveFinalizer(ba, bucketAccessFinalizer)
			if err := r.Update(ctx, ba); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure finalizer is present.
	if !controllerutil.ContainsFinalizer(ba, bucketAccessFinalizer) {
		controllerutil.AddFinalizer(ba, bucketAccessFinalizer)
		if err := r.Update(ctx, ba); err != nil {
			return ctrl.Result{}, err
		}
	}

	// If status is already granted, nothing to do.
	if granted, _ := status["accessGranted"].(bool); granted {
		return ctrl.Result{}, nil
	}

	// Fetch the referenced BucketClaim to learn backend bucket name.
	claim := &v1alpha1.BucketClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: bucketClaimName, Namespace: req.Namespace}, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}
	if !claim.Status.BucketReady || strings.TrimSpace(claim.Status.BucketName) == "" {
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	bucketName := strings.ToLower(strings.TrimSpace(claim.Status.BucketName))

	// Grant access. For now, we treat all requests as S3 + ReadWrite.
	creds, err := r.Minio.GrantAccess(ctx, bucketName, accountID, string(v1alpha1.AccessTypeReadWrite), "", "")
	if err != nil {
		logger.Error(err, "Failed to grant access", "bucketName", bucketName, "accountID", accountID)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Create credentials secret in a COSI-compatible "BucketInfo" format.
	bucketInfo := map[string]interface{}{
		"spec": map[string]interface{}{
			"s3": map[string]string{
				"accessKeyID":     creds["accessKeyID"],
				"accessSecretKey": creds["accessSecretKey"],
			},
		},
	}
	// Include endpoint when available (not required for current E2E, but helpful).
	if ep := strings.TrimSpace(creds["endpoint"]); ep != "" {
		bucketInfo["spec"].(map[string]interface{})["s3"].(map[string]string)["endpoint"] = ep
	}
	raw, err := json.Marshal(bucketInfo)
	if err != nil {
		return ctrl.Result{}, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      credentialsSecretName,
			Namespace: req.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"BucketInfo": raw,
		},
	}
	// Upsert secret (idempotent).
	existing := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: credentialsSecretName, Namespace: req.Namespace}, existing); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, secret); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	} else {
		if existing.Data == nil {
			existing.Data = map[string][]byte{}
		}
		existing.Data["BucketInfo"] = raw
		if err := r.Update(ctx, existing); err != nil {
			return ctrl.Result{}, err
		}
	}

	ba.Object["status"] = map[string]interface{}{
		"accessGranted": true,
		"accountID":     accountID,
	}
	if err := r.Status().Update(ctx, ba); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BucketAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	bucketAccessGVK := cosi.BucketAccessGVK()
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(bucketAccessGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(u).
		Complete(r)
}

