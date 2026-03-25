package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/minio"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/minio/minio-go/v7/pkg/replication"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const bucketClaimFinalizer = "objectstorage.k8s.io/finalizer"

const (
	claimConditionReady              = "Ready"
	claimReasonProvisioning          = "Provisioning"
	claimReasonBucketProvisioned     = "BucketProvisioned"
	claimReasonBucketClassNotFound   = "BucketClassNotFound"
	claimReasonBucketClassLookupFail = "BucketClassLookupFailed"
	claimReasonUnsupportedDriver     = "UnsupportedDriver"
	claimReasonCreateBucketFailed    = "CreateBucketFailed"
	claimReasonConfigureBucketFailed = "ConfigureBucketFailed"
	claimReasonGrantAccessFailed     = "GrantAccessFailed"
	claimReasonReconcileSecretFailed           = "ReconcileSecretFailed"
	claimReasonMinioCredentialSecretNotFound   = "MinioCredentialSecretNotFound"
	claimReasonMinioCredentialSecretInvalid     = "MinioCredentialSecretInvalid"
)

const maxConditionMessageRunes = 1024

// BucketClaimReconciler reconciles a BucketClaim object
type BucketClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Minio  *minio.Client

	minioMu    sync.Mutex
	minioCache map[string]*cachedMinioClient
}

type cachedMinioClient struct {
	resourceVersion string
	client          *minio.Client
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
		if apierrors.IsNotFound(err) {
			r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonBucketClassNotFound,
				fmt.Sprintf("BucketClass %q not found", claim.Spec.BucketClassName))
		} else {
			r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonBucketClassLookupFail, err.Error())
		}
		return ctrl.Result{}, err
	}
	className = class.Name

	if class.DriverName != "k8s-s3-bucket-operator" {
		log.Info("BucketClass is not supported by this operator", "DriverName", class.DriverName)
		reconcileTotal.WithLabelValues("ignored_driver").Inc()
		r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonUnsupportedDriver,
			fmt.Sprintf("BucketClass driverName %q is not handled by this operator (expected k8s-s3-bucket-operator)", class.DriverName))
		return ctrl.Result{}, nil
	}

	mc, err := r.minioClientForClass(ctx, class)
	if err != nil {
		log.Error(err, "Failed to resolve MinIO client for BucketClass")
		reconcileErrorsTotal.WithLabelValues("minio_credentials").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		if apierrors.IsNotFound(err) {
			r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonMinioCredentialSecretNotFound, err.Error())
		} else {
			r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonMinioCredentialSecretInvalid, err.Error())
		}
		return ctrl.Result{}, err
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

	if !(meta.IsStatusConditionTrue(claim.Status.Conditions, claimConditionReady) && claim.Status.Phase == "Bound") {
		r.noteClaimProvisioning(ctx, req.NamespacedName, "Provisioning bucket and access credentials")
	}

	// 1. Create Bucket
	if err := mc.CreateBucket(ctx, bucketName, region, class.ObjectLockingEnabled); err != nil {
		log.Error(err, "Failed to create bucket in MinIO")
		reconcileErrorsTotal.WithLabelValues("create_bucket").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonCreateBucketFailed, err.Error())
		return ctrl.Result{}, err
	}

	// 2. Configure advanced bucket settings
	if claim.Spec.Quota != nil {
		quotaBytes, ok := claim.Spec.Quota.AsInt64()
		if ok && quotaBytes > 0 {
			if err := mc.SetBucketQuota(ctx, bucketName, quotaBytes); err != nil {
				log.Error(err, "Failed to set bucket quota")
				reconcileErrorsTotal.WithLabelValues("set_quota").Inc()
				reconcileTotal.WithLabelValues("error").Inc()
				r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonConfigureBucketFailed, err.Error())
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
		if err := mc.SetBucketLifecycle(ctx, bucketName, lc); err != nil {
			log.Error(err, "Failed to set bucket lifecycle")
			reconcileErrorsTotal.WithLabelValues("set_lifecycle").Inc()
			reconcileTotal.WithLabelValues("error").Inc()
			r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonConfigureBucketFailed, err.Error())
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
		if err := mc.SetBucketReplication(ctx, bucketName, repCfg); err != nil {
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
	creds, err := mc.GrantAccess(ctx, bucketName, accountID, string(claim.Spec.AccessType), existingAccessKey, existingSecretKey)
	if err != nil {
		log.Error(err, "Failed to grant access")
		reconcileErrorsTotal.WithLabelValues("grant_access").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonGrantAccessFailed, err.Error())
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
		r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonReconcileSecretFailed, err.Error())
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		log.Info("Reconciled Secret", "Operation", string(op))
	}

	// 4. Update Status (re-fetch so we merge with any prior status writes this reconcile)
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		reconcileErrorsTotal.WithLabelValues("update_status").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}
	claim.Status.BucketName = bucketName
	claim.Status.Endpoint = creds["endpoint"]
	claim.Status.SecretReference = &corev1.ObjectReference{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}
	claim.Status.Phase = "Bound"
	meta.SetStatusCondition(&claim.Status.Conditions, metav1.Condition{
		Type:               claimConditionReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: claim.GetGeneration(),
		Reason:             claimReasonBucketProvisioned,
		Message:            fmt.Sprintf("Bucket %q is provisioned and credentials are available.", bucketName),
	})

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

	mc, err := r.minioClientForClass(ctx, class)
	if err != nil {
		return fmt.Errorf("minio client for finalization: %w", err)
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
	if err := mc.RevokeAccess(ctx, bucketName, accountID, accessKey); err != nil {
		log.Error(err, "Failed to revoke access during finalizer")
		// Continue anyway to try to clean up bucket if Delete policy
	}

	if class.DeletionPolicy == v1alpha1.DeletionPolicyDelete {
		log.Info("DeletionPolicy is Delete, removing bucket", "BucketName", bucketName)
		if err := mc.DeleteBucket(ctx, bucketName); err != nil {
			log.Error(err, "Failed to delete bucket")
			return err
		}
	} else {
		log.Info("DeletionPolicy is Retain, leaving bucket intact", "BucketName", bucketName)
	}

	return nil
}

func (r *BucketClaimReconciler) minioClientForClass(ctx context.Context, class *v1alpha1.BucketClass) (*minio.Client, error) {
	ref := class.MinioCredentialSecretRef
	if ref == nil || ref.Name == "" {
		return r.Minio, nil
	}
	if ref.Namespace == "" {
		return nil, fmt.Errorf("bucketClass %q: minioCredentialSecretRef.namespace is required when name is set", class.Name)
	}
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}
	if err := r.Get(ctx, key, secret); err != nil {
		return nil, err
	}
	cfg, err := minio.ConfigFromSecretData(secret.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid MinIO credential secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}
	cacheKey := ref.Namespace + "/" + ref.Name
	r.minioMu.Lock()
	defer r.minioMu.Unlock()
	if r.minioCache == nil {
		r.minioCache = make(map[string]*cachedMinioClient)
	}
	if ent := r.minioCache[cacheKey]; ent != nil && ent.resourceVersion == secret.ResourceVersion {
		return ent.client, nil
	}
	c, err := minio.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	r.minioCache[cacheKey] = &cachedMinioClient{resourceVersion: secret.ResourceVersion, client: c}
	return c, nil
}

func trimConditionMessage(msg string) string {
	runes := []rune(msg)
	if len(runes) <= maxConditionMessageRunes {
		return msg
	}
	return string(runes[:maxConditionMessageRunes]) + "…"
}

func (r *BucketClaimReconciler) noteClaimProvisioning(ctx context.Context, nn types.NamespacedName, message string) {
	if err := r.mergeClaimStatus(ctx, nn, func(c *v1alpha1.BucketClaim) {
		c.Status.Phase = "Pending"
		meta.SetStatusCondition(&c.Status.Conditions, metav1.Condition{
			Type:               claimConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: c.GetGeneration(),
			Reason:             claimReasonProvisioning,
			Message:            trimConditionMessage(message),
		})
	}); err != nil {
		log.FromContext(ctx).V(1).Info("could not set Provisioning status on BucketClaim", "error", err)
	}
}

func (r *BucketClaimReconciler) noteClaimNotReady(ctx context.Context, nn types.NamespacedName, reason, message string) {
	if err := r.mergeClaimStatus(ctx, nn, func(c *v1alpha1.BucketClaim) {
		c.Status.Phase = "Failed"
		meta.SetStatusCondition(&c.Status.Conditions, metav1.Condition{
			Type:               claimConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: c.GetGeneration(),
			Reason:             reason,
			Message:            trimConditionMessage(message),
		})
	}); err != nil {
		log.FromContext(ctx).Error(err, "failed to update BucketClaim status")
	}
}

func (r *BucketClaimReconciler) mergeClaimStatus(ctx context.Context, nn types.NamespacedName, mutate func(*v1alpha1.BucketClaim)) error {
	var c v1alpha1.BucketClaim
	if err := r.Get(ctx, nn, &c); err != nil {
		return err
	}
	if c.DeletionTimestamp != nil {
		return nil
	}
	if meta.IsStatusConditionTrue(c.Status.Conditions, claimConditionReady) && c.Status.Phase == "Bound" {
		return nil
	}
	mutate(&c)
	return r.Status().Update(ctx, &c)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BucketClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.BucketClaim{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
