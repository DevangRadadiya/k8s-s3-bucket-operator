package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/cosi"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/minio"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/provisioning"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	// EnableCOSI makes this controller also create/update COSI `Bucket` CRs and set
	// `.status.bucketReady` (needed for COSI-mode E2E assertions).
	EnableCOSI bool

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

	// If the claim was already successfully provisioned for this generation,
	// avoid repeating MinIO operations on every reconcile.
	var readyCond *metav1.Condition
	for i := range claim.Status.Conditions {
		if claim.Status.Conditions[i].Type == claimConditionReady {
			readyCond = &claim.Status.Conditions[i]
			break
		}
	}
	if readyCond != nil &&
		readyCond.Status == metav1.ConditionTrue &&
		claim.Status.Phase == "Bound" &&
		readyCond.ObservedGeneration == claim.GetGeneration() {
		return ctrl.Result{}, nil
	}

	// Fetch class
	class := &v1alpha1.BucketClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: claim.Spec.BucketClassName}, class); err != nil {
		log.Error(err, "Failed to get BucketClass", "Class", claim.Spec.BucketClassName)
		reconcileErrorsTotal.WithLabelValues("get_bucketclass").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		if apierrors.IsNotFound(err) {
			// BucketClass may be created slightly after the claim; treat as retryable.
			r.noteClaimProvisioning(ctx, req.NamespacedName,
				fmt.Sprintf("BucketClass %q not found; retrying", claim.Spec.BucketClassName))
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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
			// Secret might appear after the claim; keep claim in provisioning and retry.
			r.noteClaimProvisioning(ctx, req.NamespacedName,
				fmt.Sprintf("MinIO credentials secret not found for BucketClass %q; retrying", class.Name))
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		} else {
			r.noteClaimNotReady(ctx, req.NamespacedName, claimReasonMinioCredentialSecretInvalid, err.Error())
		}
		return ctrl.Result{}, err
	}

	if !(meta.IsStatusConditionTrue(claim.Status.Conditions, claimConditionReady) && claim.Status.Phase == "Bound") {
		r.noteClaimProvisioning(ctx, req.NamespacedName, "Provisioning bucket and access credentials")
	}
	secretName := fmt.Sprintf("%s-credentials", claim.Name)
	existingSecret := &corev1.Secret{}
	existingAccessKey := ""
	existingSecretKey := ""
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: claim.Namespace}, existingSecret); err == nil {
		existingAccessKey = strings.TrimSpace(string(existingSecret.Data["accessKeyID"]))
		existingSecretKey = strings.TrimSpace(string(existingSecret.Data["accessSecretKey"]))
	}

	creds, err := provisioning.ProvisionBucketAndGrantAccess(ctx, mc, claim, class, existingAccessKey, existingSecretKey)
	if err != nil {
		// Select stable "reason" + Prometheus "stage" from the underlying operation.
		var pErr *provisioning.ProvisioningError
		stage := "provisioning"
		reason := claimReasonConfigureBucketFailed
		bucketName := ""

		if errors.As(err, &pErr) {
			bucketName = pErr.BucketName
			switch pErr.Op {
			case provisioning.OpCreateBucket:
				stage = "create_bucket"
				reason = claimReasonCreateBucketFailed
			case provisioning.OpSetQuota:
				stage = "set_quota"
				reason = claimReasonConfigureBucketFailed
			case provisioning.OpSetLifecycle:
				stage = "set_lifecycle"
				reason = claimReasonConfigureBucketFailed
			case provisioning.OpGrantAccess:
				stage = "grant_access"
				reason = claimReasonGrantAccessFailed
			}
		}

		log.Error(err, "Failed to provision bucket and grant access")
		reconcileErrorsTotal.WithLabelValues(stage).Inc()
		reconcileTotal.WithLabelValues("error").Inc()

		if provisioning.IsTransientMinioError(err) {
			// Keep message deterministic so conditions remain parseable by E2E tests.
			transientMsg := fmt.Sprintf("Transient MinIO error during %s; bucket=%q; retrying: %v", stage, bucketName, err)
			if pErr != nil {
				switch pErr.Op {
				case provisioning.OpCreateBucket:
					transientMsg = fmt.Sprintf("Transient MinIO error creating bucket %q; retrying: %v", bucketName, err)
				case provisioning.OpSetQuota:
					transientMsg = fmt.Sprintf("Transient MinIO error setting quota for bucket %q; retrying: %v", bucketName, err)
				case provisioning.OpSetLifecycle:
					transientMsg = fmt.Sprintf("Transient MinIO error setting lifecycle for bucket %q; retrying: %v", bucketName, err)
				case provisioning.OpGrantAccess:
					transientMsg = fmt.Sprintf("Transient MinIO error granting access for bucket %q; retrying: %v", bucketName, err)
				}
			}

			r.noteClaimProvisioning(ctx, req.NamespacedName, transientMsg)
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}

		r.noteClaimNotReady(ctx, req.NamespacedName, reason, err.Error())
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
		secret.StringData["accessKeyID"] = creds.AccessKeyID
		secret.StringData["accessSecretKey"] = creds.AccessSecretKey
		secret.StringData["bucketName"] = creds.BucketName
		secret.StringData["endpoint"] = creds.Endpoint
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
	claim.Status.BucketName = creds.BucketName
	claim.Status.Endpoint = creds.Endpoint
	claim.Status.SecretReference = &corev1.ObjectReference{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}
	claim.Status.Phase = "Bound"
	claim.Status.BucketReady = true
	meta.SetStatusCondition(&claim.Status.Conditions, metav1.Condition{
		Type:               claimConditionReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: claim.GetGeneration(),
		Reason:             claimReasonBucketProvisioned,
		Message:            fmt.Sprintf("Bucket %q is provisioned and credentials are available.", creds.BucketName),
	})

	if err := r.Status().Update(ctx, claim); err != nil {
		log.Error(err, "Failed to update BucketClaim status")
		reconcileErrorsTotal.WithLabelValues("update_status").Inc()
		reconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	// In COSI mode, create the corresponding COSI Bucket object (cluster-scoped) so
	// E2E can assert `.status.bucketReady`.
	if r.EnableCOSI {
		if err := r.ensureCosiBucket(ctx, claim, class); err != nil {
			log.Error(err, "Failed to reconcile COSI Bucket")
			reconcileErrorsTotal.WithLabelValues("reconcile_cosi_bucket").Inc()
			reconcileTotal.WithLabelValues("error").Inc()
			return ctrl.Result{}, err
		}
	}

	bucketsBoundTotal.WithLabelValues(class.Name).Inc()
	reconcileTotal.WithLabelValues("success").Inc()
	log.Info("Successfully reconciled BucketClaim", "BucketName", creds.BucketName)
	return ctrl.Result{}, nil
}

func (r *BucketClaimReconciler) ensureCosiBucket(ctx context.Context, claim *v1alpha1.BucketClaim, class *v1alpha1.BucketClass) error {
	bucketName := strings.TrimSpace(claim.Status.BucketName)
	if bucketName == "" {
		return nil
	}

	// Build the desired COSI Bucket object as unstructured data so we don't need to couple
	// our core controller to the external COSI Go types.
	bucketGVK := cosi.BucketGVK()

	bu := &unstructured.Unstructured{}
	bu.SetGroupVersionKind(bucketGVK)
	bu.SetName(bucketName)

	protocols := claim.Spec.Protocols
	if len(protocols) == 0 {
		protocols = []string{"S3"}
	}

	spec := map[string]interface{}{
		"bucketClaim": map[string]interface{}{
			"apiVersion": cosi.ObjectStorageAPIGroup + "/" + cosi.ObjectStorageAPIVersion,
			"kind":       "BucketClaim",
			"name":       claim.Name,
			"namespace":  claim.Namespace,
		},
		"bucketClassName": claim.Spec.BucketClassName,
		"driverName":       class.DriverName,
		"deletionPolicy":  string(class.DeletionPolicy),
		"protocols":       protocols,
	}
	if class.Parameters != nil && len(class.Parameters) > 0 {
		spec["parameters"] = class.Parameters
	}

	status := map[string]interface{}{
		"bucketReady": true,
		"bucketID":    bucketName,
	}

	if err := r.Get(ctx, client.ObjectKey{Name: bucketName}, bu); err != nil {
		if apierrors.IsNotFound(err) {
			bu.Object["spec"] = spec
			bu.Object["status"] = status
			return r.Create(ctx, bu)
		}
		return err
	}

	// Only status is required for E2E right now.
	bu.Object["status"] = status
	return r.Status().Update(ctx, bu)
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

	accessKey := ""
	if claim.Status.SecretReference != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: claim.Status.SecretReference.Name, Namespace: claim.Status.SecretReference.Namespace}, secret); err == nil {
			accessKey = strings.TrimSpace(string(secret.Data["accessKeyID"]))
		}
	}
	return provisioning.RevokeAccessAndMaybeDeleteBucket(ctx, mc, claim, class, accessKey)
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
		c.Status.BucketReady = false
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
		c.Status.BucketReady = false
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
