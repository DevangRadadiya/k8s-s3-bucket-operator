package resolve

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	v1alpha1 "github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/backend"
	awsbackend "github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/backend/aws"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/minio"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resolver returns a backend.Provider for a BucketClass (MinIO today; AWS when implemented).
type Resolver struct {
	Client       client.Client
	DefaultMinio *minio.Client

	mu    sync.Mutex
	cache map[string]*cachedMinio

	awsMu    sync.Mutex
	awsCache map[string]*cachedAWS
}

type cachedMinio struct {
	resourceVersion string
	client          *minio.Client
}

type cachedAWS struct {
	resourceVersion string
	provider        *awsbackend.Provider
}

// NewResolver builds a resolver. defaultMinio is used when the class has no per-class credential secret.
func NewResolver(c client.Client, defaultMinio *minio.Client) *Resolver {
	return &Resolver{Client: c, DefaultMinio: defaultMinio}
}

// ProviderForClass returns the storage provider for the given class.
func (r *Resolver) ProviderForClass(ctx context.Context, class *v1alpha1.BucketClass) (backend.Provider, error) {
	if r == nil {
		return nil, fmt.Errorf("resolver is nil")
	}
	kind, err := backend.KindForBucketClass(class)
	if err != nil {
		return nil, err
	}

	switch kind {
	case backend.KindMinIO:
		return r.minioProviderForClass(ctx, class)
	case backend.KindAWS:
		return r.awsProviderForClass(ctx, class)
	default:
		return nil, fmt.Errorf("%w: %s", backend.ErrNotImplemented, kind)
	}
}

func (r *Resolver) minioProviderForClass(ctx context.Context, class *v1alpha1.BucketClass) (backend.Provider, error) {
	ref := class.MinioCredentialSecretRef
	if ref == nil || ref.Name == "" {
		if r.DefaultMinio == nil {
			return nil, fmt.Errorf("no default MinIO client configured")
		}
		return r.DefaultMinio, nil
	}
	if ref.Namespace == "" {
		return nil, fmt.Errorf("bucketClass %q: minioCredentialSecretRef.namespace is required when name is set", class.Name)
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		return nil, err
	}
	cfg, err := minio.ConfigFromSecretData(secret.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid MinIO credential secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}

	cacheKey := ref.Namespace + "/" + ref.Name
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == nil {
		r.cache = make(map[string]*cachedMinio)
	}
	if ent := r.cache[cacheKey]; ent != nil && ent.resourceVersion == secret.ResourceVersion {
		return ent.client, nil
	}
	c, err := minio.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	r.cache[cacheKey] = &cachedMinio{resourceVersion: secret.ResourceVersion, client: c}
	return c, nil
}

func (r *Resolver) awsProviderForClass(ctx context.Context, class *v1alpha1.BucketClass) (backend.Provider, error) {
	ref := class.AwsCredentialSecretRef
	if ref == nil || ref.Name == "" {
		return nil, fmt.Errorf("bucketClass %q: awsCredentialSecretRef is required when backend=AWS", class.Name)
	}
	if ref.Namespace == "" {
		return nil, fmt.Errorf("bucketClass %q: awsCredentialSecretRef.namespace is required when name is set", class.Name)
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		return nil, err
	}
	cfg, err := awsbackend.ConfigFromSecretData(secret.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid AWS credential secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}

	// Optionally load a bucket policy document referenced by the class.
	policyKey := ""
	policyRV := ""
	policyDoc := []byte(nil)
	if pr := class.BucketPolicyRef; pr != nil && strings.TrimSpace(pr.Name) != "" {
		kind := strings.TrimSpace(string(pr.Kind))
		if kind == "" {
			kind = "ConfigMap"
		}
		if strings.TrimSpace(pr.Namespace) == "" || strings.TrimSpace(pr.Key) == "" {
			return nil, fmt.Errorf("bucketClass %q: bucketPolicyRef requires namespace, name, and key", class.Name)
		}
		policyKey = kind + ":" + pr.Namespace + "/" + pr.Name + "#" + pr.Key

		switch kind {
		case "ConfigMap":
			cm := &corev1.ConfigMap{}
			if err := r.Client.Get(ctx, types.NamespacedName{Namespace: pr.Namespace, Name: pr.Name}, cm); err != nil {
				return nil, err
			}
			policyRV = cm.ResourceVersion
			raw := strings.TrimSpace(cm.Data[pr.Key])
			if raw == "" {
				return nil, fmt.Errorf("bucketClass %q: bucketPolicyRef %s missing data[%q]", class.Name, pr.Namespace+"/"+pr.Name, pr.Key)
			}
			policyDoc = []byte(raw)
		case "Secret":
			ps := &corev1.Secret{}
			if err := r.Client.Get(ctx, types.NamespacedName{Namespace: pr.Namespace, Name: pr.Name}, ps); err != nil {
				return nil, err
			}
			policyRV = ps.ResourceVersion
			raw := ps.Data[pr.Key]
			if len(raw) == 0 {
				return nil, fmt.Errorf("bucketClass %q: bucketPolicyRef %s missing data[%q]", class.Name, pr.Namespace+"/"+pr.Name, pr.Key)
			}
			policyDoc = raw
		default:
			return nil, fmt.Errorf("bucketClass %q: bucketPolicyRef.kind must be ConfigMap or Secret", class.Name)
		}
		// Validate JSON early so reconcile fails with a clear error.
		var tmp map[string]any
		if err := json.Unmarshal(policyDoc, &tmp); err != nil {
			return nil, fmt.Errorf("bucketClass %q: bucketPolicyRef JSON invalid: %w", class.Name, err)
		}
	}

	cacheKey := ref.Namespace + "/" + ref.Name + "|" + policyKey
	r.awsMu.Lock()
	defer r.awsMu.Unlock()
	if r.awsCache == nil {
		r.awsCache = make(map[string]*cachedAWS)
	}
	combinedRV := secret.ResourceVersion + "|" + policyRV
	if ent := r.awsCache[cacheKey]; ent != nil && ent.resourceVersion == combinedRV {
		return ent.provider, nil
	}

	p, err := awsbackend.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	p = p.WithBucketPolicyDocument(policyDoc)
	r.awsCache[cacheKey] = &cachedAWS{resourceVersion: combinedRV, provider: p}
	return p, nil
}
