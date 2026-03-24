package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/minio"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/minio/minio-go/v7/pkg/replication"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const bucketClaimFinalizer = "objectstorage.k8s.io/finalizer"

// BucketClaimReconciler reconciles a BucketClaim object
type BucketClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Minio  *minio.Client
}

func (r *BucketClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	className := "unknown"
	start := time.Now()
	defer func() {
		reconcileDurationSeconds.WithLabelValues(className).Observe(time.Since(start).Seconds())
	}()

	// Fetch the BucketClaim instance
	claim := &v1alpha1.BucketClaim{}
	err := r.Get(ctx, req.NamespacedName, claim)
	if err != nil {
		if apierrors.IsNotFound(err) {
			reconcileTotal.WithLabelValues("not_found").Inc()
			return ctrl.Result{}, nil
		}
		reconcileErrorsTotal.WithLabelValues("get_claim").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	// Check if the Resource is marked to be deleted
	isMarkedToBeDeleted := claim.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(claim, bucketClaimFinalizer) {
			if err := r.finalizeBucketClaim(ctx, claim); err != nil {
				log.Error(err, "Failed to finalize BucketClaim")
				reconcileErrorsTotal.WithLabelValues("finalize").Inc()
				reconcileTotal.WithLabelValues("error").Inc()
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(claim, bucketClaimFinalizer)
			err := r.Update(ctx, claim)
			if err != nil {
				reconcileErrorsTotal.WithLabelValues("remove_finalizer").Inc()
				reconcileTotal.WithLabelValues("error").Inc()
				return ctrl.Result{}, err
			}
		}
		reconcileTotal.WithLabelValues("deleted").Inc()
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(claim, bucketClaimFinalizer) {
		controllerutil.AddFinalizer(claim, bucketClaimFinalizer)
		err = r.Update(ctx, claim)
		if err != nil {
			reconcileErrorsTotal.WithLabelValues("add_finalizer").Inc()
			reconcileTotal.WithLabelValues("error").Inc()
			return ctrl.Result{}, err
		}
	}

	// Fetch class
	class := &v1alpha1.BucketClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: claim.Spec.BucketClassName}, class); err != nil {
		log.Error(err, "Failed to get BucketClass", "Class", claim.Spec.BucketClassName)
		reconcileErrorsTotal.WithLabelValues("get_bucketclass").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}
	className = class.Name

	if class.DriverName != "k8s-s3-bucket-operator" {
		log.Info("BucketClass is not supported by this operator", "DriverName", class.DriverName)
		reconcileTotal.WithLabelValues("ignored_driver").Inc()
		return ctrl.Result{}, nil
	}

	// Determine region
	region := class.Parameters["region"]
	if region == "" {
		region = "us-east-1"
	}

	// Determine bucket name
	bucketName := claim.Spec.BucketName
	if bucketName == "" {
		bucketName = fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
	}

	// 1. Create Bucket
	if err := r.Minio.CreateBucket(ctx, bucketName, region, class.ObjectLockingEnabled); err != nil {
		log.Error(err, "Failed to create bucket in MinIO")
		reconcileErrorsTotal.WithLabelValues("create_bucket").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	// 2. Configure advanced bucket settings
	if claim.Spec.Quota != nil {
		quotaBytes, ok := claim.Spec.Quota.AsInt64()
		if ok && quotaBytes > 0 {
			if err := r.Minio.SetBucketQuota(ctx, bucketName, quotaBytes); err != nil {
				log.Error(err, "Failed to set bucket quota")
				reconcileErrorsTotal.WithLabelValues("set_quota").Inc()
				reconcileTotal.WithLabelValues("error").Inc()
				return ctrl.Result{}, err
			}
		}
	}

	if len(claim.Spec.LifecycleRules) > 0 {
		lc := &lifecycle.Configuration{}
		for _, rule := range claim.Spec.LifecycleRules {
			lc.Rules = append(lc.Rules, lifecycle.Rule{
				ID:     rule.ID,
				Status: rule.Status,
				RuleFilter: lifecycle.Filter{
					Prefix: rule.Prefix,
				},
				Expiration: lifecycle.Expiration{
					Days: lifecycle.ExpirationDays(rule.Expiration.Days),
				},
			})
		}
		if err := r.Minio.SetBucketLifecycle(ctx, bucketName, lc); err != nil {
			log.Error(err, "Failed to set bucket lifecycle")
			reconcileErrorsTotal.WithLabelValues("set_lifecycle").Inc()
			reconcileTotal.WithLabelValues("error").Inc()
			return ctrl.Result{}, err
		}
	}

	if claim.Spec.ReplicationTarget != nil {
		repCfg := replication.Config{
			Rules: []replication.Rule{
				{
					Status: "Enabled",
					Destination: replication.Destination{
						Bucket: "arn:aws:s3:::" + claim.Spec.ReplicationTarget.BucketName,
					},
				},
			},
		}
		if err := r.Minio.SetBucketReplication(ctx, bucketName, repCfg); err != nil {
			log.Error(err, "Failed to set bucket replication")
		}
	}

	// 3. Grant Access
	// Account ID is just namespace/name for isolating policies
	accountID := fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
	secretName := fmt.Sprintf("%s-credentials", claim.Name)
	existingSecret := &corev1.Secret{}
	existingAccessKey := ""
	existingSecretKey := ""
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: claim.Namespace}, existingSecret); err == nil {
		existingAccessKey = strings.TrimSpace(string(existingSecret.Data["accessKeyID"]))
		existingSecretKey = strings.TrimSpace(string(existingSecret.Data["accessSecretKey"]))
	}
	creds, err := r.Minio.GrantAccess(ctx, bucketName, accountID, string(claim.Spec.AccessType), existingAccessKey, existingSecretKey)
	if err != nil {
		log.Error(err, "Failed to grant access")
		reconcileErrorsTotal.WithLabelValues("grant_access").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	// 3. Create Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: claim.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.StringData == nil {
			secret.StringData = make(map[string]string)
		}
		secret.StringData["accessKeyID"] = creds["accessKeyID"]
		secret.StringData["accessSecretKey"] = creds["accessSecretKey"]
		secret.StringData["bucketName"] = creds["bucketName"]
		secret.StringData["endpoint"] = creds["endpoint"]
		// Set owner reference to the claim automatically cleans up the secret when claim is deleted
		return controllerutil.SetControllerReference(claim, secret, r.Scheme)
	})

	if err != nil {
		log.Error(err, "Failed to reconcile Secret")
		reconcileErrorsTotal.WithLabelValues("reconcile_secret").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		log.Info("Reconciled Secret", "Operation", string(op))
	}

	// 4. Update Status
	claim.Status.BucketName = bucketName
	claim.Status.Endpoint = creds["endpoint"]
	claim.Status.SecretReference = &corev1.ObjectReference{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}
	claim.Status.Phase = "Bound"

	if err := r.Status().Update(ctx, claim); err != nil {
		log.Error(err, "Failed to update BucketClaim status")
		reconcileErrorsTotal.WithLabelValues("update_status").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	bucketsBoundTotal.WithLabelValues(class.Name).Inc()
	reconcileTotal.WithLabelValues("success").Inc()
	log.Info("Successfully reconciled BucketClaim", "BucketName", bucketName)
	return ctrl.Result{}, nil
}

func (r *BucketClaimReconciler) finalizeBucketClaim(ctx context.Context, claim *v1alpha1.BucketClaim) error {
	log := log.FromContext(ctx)

	// Fetch class for deletion policy
	class := &v1alpha1.BucketClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: claim.Spec.BucketClassName}, class); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("BucketClass not found, skipping finalization steps")
			return nil
		}
		return err
	}

	bucketName := claim.Status.BucketName
	if bucketName == "" {
		bucketName = claim.Spec.BucketName
		if bucketName == "" {
			bucketName = fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
		}
	}

	accountID := fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
	accessKey := ""
	if claim.Status.SecretReference != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: claim.Status.SecretReference.Name, Namespace: claim.Status.SecretReference.Namespace}, secret); err == nil {
			accessKey = strings.TrimSpace(string(secret.Data["accessKeyID"]))
		}
	}

	// Always revoke access to delete the minio policy/user
	if err := r.Minio.RevokeAccess(ctx, bucketName, accountID, accessKey); err != nil {
		log.Error(err, "Failed to revoke access during finalizer")
		// Continue anyway to try to clean up bucket if Delete policy
	}

	if class.DeletionPolicy == v1alpha1.DeletionPolicyDelete {
		log.Info("DeletionPolicy is Delete, removing bucket", "BucketName", bucketName)
		if err := r.Minio.DeleteBucket(ctx, bucketName); err != nil {
			log.Error(err, "Failed to delete bucket")
			return err
		}
	} else {
		log.Info("DeletionPolicy is Retain, leaving bucket intact", "BucketName", bucketName)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BucketClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.BucketClaim{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
