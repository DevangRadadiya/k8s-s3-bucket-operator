# User guide — k8s-s3-bucket-operator

This guide describes how to install the operator, configure **BucketClass** and **BucketClaim**, and use the **enterprise-style** fields (quotas, lifecycle, object lock settings, IAM access type, replication target, deletion policy).

For a minimal install path, see the [README](../README.md) **Quick start**. For quick Q&A, see [`FAQ.md`](FAQ.md).

---

## Table of contents

1. [Concepts](#concepts)
2. [Installation](#installation)
3. [BucketClass reference](#bucketclass-reference)
4. [BucketClaim reference](#bucketclaim-reference)
5. [Secret created for your app](#secret-created-for-your-app)
6. [Enterprise features (details)](#enterprise-features-details)
7. [Verification with MinIO Client (`mc`)](#verification-with-minio-client-mc)
8. [Troubleshooting](#troubleshooting)

---

## Concepts

| Resource | Scope | Who creates it | Purpose |
|----------|-------|----------------|---------|
| **BucketClass** | Cluster | Cluster admin | Binds a class name to this operator (`driverName`), MinIO region/parameters, **deletion policy**, and optional **object lock / retention** defaults. |
| **BucketClaim** | Namespaced | App team | Requests a bucket; operator creates the bucket, optional **quota / lifecycle / replication**, generates **IAM-style** access, and writes a **Secret**. |

The operator talks to MinIO using environment variables on the deployment (see `deploy/operator.yaml` → `Secret` `minio-credentials`).

---

## Installation

### Prerequisites

- Kubernetes 1.25+ (or OpenShift 4.12+)
- A reachable MinIO (or S3-compatible) endpoint
- `kubectl` configured for your cluster

### Deploy CRDs and operator

From the repository root (or use raw URLs to the same files on `main`):

```bash
kubectl apply -f deploy/objectstorage.k8s.io_bucketclasses.yaml
kubectl apply -f deploy/objectstorage.k8s.io_bucketclaims.yaml
kubectl apply -f deploy/operator.yaml
```

Or use Make:

```bash
make deploy
```

Or apply everything in one step with Kustomize (from a clone):

```bash
kubectl apply -k deploy/
```

**Important:** Edit the `minio-credentials` Secret in `deploy/operator.yaml` (or patch after apply) so `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, and `MINIO_SECRET_KEY` match your MinIO deployment.

### Image tag

Published images: `ghcr.io/devangradadiya/k8s-s3-bucket-operator:latest` (and `:main` for the latest `main` branch build from CI).

Default manifests use **`latest`**. For reproducible tests against a specific CI build, you can temporarily set the image to `:main` or a release tag.

### High availability

The default deployment runs **2 replicas** with **`--leader-elect=true`** so only one reconciler is active at a time.

A **PodDisruptionBudget** (`minAvailable: 1`) is included in `deploy/operator.yaml` so voluntary disruptions (node drains) do not take all replicas down at once.

### Monitoring (Prometheus)

Metrics are exposed on **port 8080** at path **`/metrics`**. A ClusterIP **Service** (`k8s-s3-bucket-operator-metrics`) is defined for scraping.

See **[MONITORING.md](MONITORING.md)** for metric names, a Grafana dashboard import (`deploy/grafana-dashboard.json`), and `ServiceMonitor` hints.

### API stability (`v1alpha1`)

`objectstorage.k8s.io/v1alpha1` may change before a **beta/GA** API. Pin operator and chart versions for production, and read release notes when upgrading.

### Production image tags

`:latest` and `:main` are convenient for development. For production, prefer a **release tag** (for example `v0.2.0`) or an **image digest** so rollouts are reproducible.

---

## BucketClass reference

Example:

```yaml
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: minio-standard
driverName: k8s-s3-bucket-operator
deletionPolicy: Delete          # Delete | Retain
objectLockingEnabled: true      # optional; enables object lock at bucket creation
retentionMode: GOVERNANCE       # optional; GOVERNANCE | COMPLIANCE (used when locking is enabled)
retentionDays: 30               # optional; align with your compliance process
parameters:
  region: "us-east-1"
```

| Field | Type | Description |
|-------|------|-------------|
| `driverName` | string | Must be `k8s-s3-bucket-operator` for this operator to reconcile claims using this class. |
| `deletionPolicy` | `Delete` / `Retain` | On **BucketClaim** deletion: remove MinIO user/policy always; **delete** or **retain** the bucket. |
| `objectLockingEnabled` | bool | If true, bucket is created with **object locking** enabled (cannot be enabled later on an existing bucket without lock). |
| `retentionMode` | `GOVERNANCE` / `COMPLIANCE` | Declared default retention mode for documentation / future enforcement; align with your compliance process. |
| `retentionDays` | int | Declared retention duration in days; align with your compliance process. |
| `parameters.region` | string | S3 region passed to MinIO bucket creation (default `us-east-1` if empty). |

---

## BucketClaim reference

Example with optional enterprise fields:

```yaml
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: my-app-images
  namespace: my-app
spec:
  bucketClassName: minio-standard
  bucketName: my-custom-bucket        # optional; default is <namespace>-<claim-name>
  protocols:
    - S3
  quota: "50Gi"                       # optional; hard quota on the bucket
  accessType: ReadOnly                # ReadWrite (default) | ReadOnly
  lifecycleRules:
    - id: expire-backups
      status: Enabled                 # Enabled | Disabled
      prefix: "backups/"
      expiration:
        days: 30
  replicationTarget:                  # optional; replication rule on source bucket
    endpoint: "minio-other.example.com:9000"
    bucketName: "replica-bucket"
    accessKey: "<target-admin-or-replication-user>"
    secretKey: "<secret>"
    useSSL: false
```

| Field | Type | Description |
|-------|------|-------------|
| `bucketClassName` | string | **Required.** Name of a `BucketClass`. |
| `bucketName` | string | Optional explicit bucket name. |
| `protocols` | `[]string` | e.g. `S3`. |
| `quota` | quantity string | Optional Kubernetes-style quantity (`50Gi`, `100Mi`, …). Maps to MinIO **hard quota**. |
| `accessType` | `ReadWrite` / `ReadOnly` | IAM policy attached to the generated user. Default `ReadWrite`. |
| `lifecycleRules` | array | S3-style lifecycle rules (prefix + expiration days). Applied via MinIO SDK. |
| `replicationTarget` | object | Target endpoint, bucket, and credentials for configuring **replication** on the source bucket. Requires a valid MinIO replication setup on the target. |

---

## Secret created for your app

The operator creates a Secret named **`<claim-name>-credentials`** in the claim’s namespace with:

| Key | Description |
|-----|-------------|
| `accessKeyID` | MinIO access key for this claim |
| `accessSecretKey` | MinIO secret key |
| `bucketName` | Bucket name |
| `endpoint` | S3 endpoint URL (from the operator’s MinIO client) |

Mount or reference these in your `Deployment` / `Pod` like any other Secret.

---

## Enterprise features (details)

### 1. Bucket lifecycle

- Configured from `spec.lifecycleRules`.
- Uses MinIO/S3 lifecycle APIs (`SetBucketLifecycle`).
- Verify with: `mc ilm rule ls ALIAS/BUCKET` (see below).

### 2. Quota / size limit

- Configured from `spec.quota`.
- Uses MinIO Admin API **hard quota**.
- Quota enforcement may depend on MinIO scanner timing (not always instantaneous).
- Verify with: `mc quota info ALIAS/BUCKET`.

### 3. Object lock (WORM) at bucket level

- Controlled by **BucketClass** `objectLockingEnabled` at **bucket creation** time.
- Retention fields (`retentionMode`, `retentionDays`) are part of the API for class-level documentation; full default retention configuration may require additional MinIO steps depending on your version and policy.

### 4. Custom IAM-style policies

- `accessType: ReadWrite` — get/put/delete/list on the bucket.
- `accessType: ReadOnly` — get/list (read-only) on the bucket.

### 5. Replication

- **Status:** advanced / **partially supported** — the operator applies a minimal replication configuration via the MinIO API; end-to-end replication still depends on your MinIO topology, versioning, and remote credentials.
- `replicationTarget` triggers `SetBucketReplication` on the **source** bucket.
- The target cluster must be reachable and configured to accept replication; you may need extra MinIO configuration (service accounts, replication ARNs) beyond this operator’s minimal rule.
- Validate against the **MinIO version** you run; treat this field as experimental until you have a tested reference architecture.
- Treat this as an **advanced** feature and validate in your environment.

### 6. Deletion policy

- **Retain:** claim and secret can be removed; bucket may remain in MinIO (and credentials revoked on finalizer).
- **Delete:** operator attempts to delete the bucket when the claim is deleted (subject to MinIO rules, e.g. non-empty bucket or object lock).

---

## Verification with MinIO Client (`mc`)

From a pod that has `mc` and network access to MinIO (e.g. the MinIO container in tests):

```bash
mc alias set myminio http://<minio-host>:9000 <admin-user> <admin-password>
mc quota info myminio/<bucket>
mc ilm rule ls myminio/<bucket>
```

Note: `mc ilm ls` was superseded by `mc ilm rule ls` on newer MinIO clients.

---

## Troubleshooting

| Symptom | Check |
|---------|--------|
| Claim stuck / not Bound | `kubectl describe bucketclaim -n <ns> <name>`; operator logs: `kubectl logs -n k8s-s3-bucket-operator deploy/k8s-s3-bucket-operator` |
| CrashLoopBackOff | Ensure CRDs match the operator version; ensure `MINIO_*` env vars are correct. |
| Quota / ILM empty in `mc` | Confirm the operator image matches the code you expect; wait for scanner; use `mc ilm rule ls` not deprecated commands. |
| Replication errors | Validate target credentials, bucket existence, and MinIO replication docs for your version. |

---

## Further reading

- [README](../README.md) — overview and quick start
- [CONTRIBUTING.md](../CONTRIBUTING.md)
- Sample manifests: `config/samples/`
- E2E script: `test/e2e/run-e2e.sh`

### GitHub Wiki (optional)

You can **mirror** this file into [GitHub Wiki](https://docs.github.com/en/communities/documenting-your-project-with-wikis/about-wikis) for discoverability. Wikis are **not** versioned with git by default; keeping **`docs/` in the repo** remains the source of truth for each release.
