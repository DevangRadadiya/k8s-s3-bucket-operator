# k8s-s3-bucket-operator

Kubernetes operator for provisioning and managing S3-compatible buckets (MinIO first) through `BucketClass` and `BucketClaim` resources.

**TL;DR:** Apply CRDs + operator YAML → set the `minio-credentials` Secret → apply `BucketClass` + `BucketClaim` → use the generated Secret `<claim-name>-credentials` in your app.

**Repository:** [github.com/DevangRadadiya/k8s-s3-bucket-operator](https://github.com/DevangRadadiya/k8s-s3-bucket-operator)

**Default container image:** `ghcr.io/devangradadiya/k8s-s3-bucket-operator:latest` (see `deploy/operator.yaml`)

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.25%2B-326CE5?logo=kubernetes)](https://kubernetes.io)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev)
[![OpenShift Ready](https://img.shields.io/badge/OpenShift-Ready-EE0000?logo=redhatopenshift)](https://www.redhat.com/en/technologies/cloud-computing/openshift)
[![Publish Docker image](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/actions/workflows/docker-publish.yml)
[![Publish Helm chart](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/actions/workflows/helm-publish.yml/badge.svg)](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/actions/workflows/helm-publish.yml)
[![Test](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/actions/workflows/test.yml/badge.svg)](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/actions/workflows/test.yml)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/k8s-s3-bucket-operator)](https://artifacthub.io/packages/search?repo=k8s-s3-bucket-operator)

## Support 💬

| Need | Where |
|------|--------|
| Bug report | [Issues](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/issues) (use templates) |
| Questions | [Discussions](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/discussions) (if enabled) or open a Q&A issue |
| Security | [`SECURITY.md`](SECURITY.md) — **do not** file public issues for vulnerabilities |

## What this operator does 🚀

- Reconciles `BucketClaim` objects and creates buckets in MinIO.
- Can also provision buckets in AWS S3 when `BucketClass.backend: AWS` is used.
- Generates per-claim credentials and stores them in namespaced Kubernetes Secrets.
- Applies claim-level controls like quota, lifecycle, access type, and replication target.
- Enforces class-level settings like deletion policy and object lock at bucket creation.

## Key features ✨

- **Automatic bucket provisioning** from `BucketClaim`
- **Credential Secret generation** (`<claim-name>-credentials`)
- **Quota management** (`spec.quota`, e.g. `50Gi`)
- **Lifecycle rules** (`spec.lifecycleRules`)
- **Access policy type** (`spec.accessType`: `ReadWrite` / `ReadOnly`)
- **Replication target config** (`spec.replicationTarget`, advanced)
- **Deletion policy** (`BucketClass.deletionPolicy`: `Delete` / `Retain`)
- **Object lock toggle at creation** (`BucketClass.objectLockingEnabled`)
- **HA deployment support** (2 replicas + leader election)
- **OpenShift-compatible manifests** under `deploy/openshift/`
- **Helm chart** — [`charts/k8s-s3-bucket-operator`](charts/k8s-s3-bucket-operator) (published to **GHCR OCI** by CI)
- **AWS backend (MVP)** — `BucketClass.backend: AWS` with secure bucket defaults (public access block, ACL disable, default encryption, TLS-only policy) and optional custom bucket policy attachment (`bucketPolicyRef`). See [`docs/AWS.md`](docs/AWS.md).

## Documentation 📚

| Doc | Location | Purpose |
|---|---|---|
| User guide | [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md) | Full install + CRD behavior + troubleshooting |
| AWS backend | [`docs/AWS.md`](docs/AWS.md) | S3 + IAM setup, IAM policy, smoke test |
| FAQ | [`docs/FAQ.md`](docs/FAQ.md) | Short answers to common problems |
| Try locally | [`docs/TRY_LOCALLY.md`](docs/TRY_LOCALLY.md) | Minimal local cluster + MinIO path |
| Release checklist | [`docs/RELEASE_CHECKLIST.md`](docs/RELEASE_CHECKLIST.md) | Tag/release process and verification |
| Contributing | [`CONTRIBUTING.md`](CONTRIBUTING.md) | Dev workflow and PR guidelines |
| Changelog | [`CHANGELOG.md`](CHANGELOG.md) | User-visible project changes |
| Security policy | [`SECURITY.md`](SECURITY.md) | Private vulnerability reporting process |
| Code of conduct | [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) | Contributor Covenant 3.0 |
| Helm charts | [`charts/README.md`](charts/README.md) | OCI install, values, CRD notes |
| Artifact Hub | [`docs/ARTIFACT_HUB.md`](docs/ARTIFACT_HUB.md) | Register & scan the OCI repo |
| Public roadmap | [`docs/ROADMAP.md`](docs/ROADMAP.md) | Planned delivery waves and priorities |
| Capability matrix | [`docs/CAPABILITY_MATRIX.md`](docs/CAPABILITY_MATRIX.md) | Supported-now vs planned feature/backends |
| Monitoring | [`docs/MONITORING.md`](docs/MONITORING.md) | Prometheus metrics and Grafana dashboard |
| Examples | [`examples/`](examples/) | PostgreSQL backup, uploads, log archival |

## Quick start (for users) ⚡

### Prerequisites

- Kubernetes 1.25+ (or OpenShift 4.12+)
- A reachable MinIO endpoint
- `kubectl` (or `oc`)

### 1) Deploy CRDs and operator

Use published manifests from this repository (no clone, no build):

```bash
REPO_URL="https://raw.githubusercontent.com/DevangRadadiya/k8s-s3-bucket-operator/main"
kubectl apply -f "${REPO_URL}/deploy/objectstorage.k8s.io_bucketclasses.yaml"
kubectl apply -f "${REPO_URL}/deploy/objectstorage.k8s.io_bucketclaims.yaml"
kubectl apply -f "${REPO_URL}/deploy/operator.yaml"
```

**Alternative (Kustomize, from a clone):** applies the same three manifests in one step:

```bash
git clone https://github.com/DevangRadadiya/k8s-s3-bucket-operator.git
cd k8s-s3-bucket-operator
kubectl apply -k deploy/
```

**Alternative (Helm 3.8+, OCI):** after CI has published the chart (see badge above):

```bash
helm install k8s-s3-bucket-operator oci://ghcr.io/devangradadiya/helm-charts/k8s-s3-bucket-operator \
  --version 0.1.2 \
  --namespace k8s-s3-bucket-operator \
  --create-namespace
```

Override MinIO settings with `--set` / `-f values.yaml`; for production use `minio.existingSecret`. Details: [`charts/README.md`](charts/README.md).

**MinIO credentials**

- **kubectl / Kustomize:** set keys on the `minio-credentials` Secret in `k8s-s3-bucket-operator` (`MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_USE_SSL`), then restart the Deployment.
- **Helm:** the chart can create that Secret from `values.yaml` (default placeholders). Replace with real values or `minio.existingSecret` before relying on it in production.

Example (kubectl path — replace values):

```bash
kubectl -n k8s-s3-bucket-operator create secret generic minio-credentials \
  --from-literal=MINIO_ENDPOINT="minio.minio-ns.svc.cluster.local:9000" \
  --from-literal=MINIO_ACCESS_KEY="YOUR_KEY" \
  --from-literal=MINIO_SECRET_KEY="YOUR_SECRET" \
  --from-literal=MINIO_USE_SSL="false" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n k8s-s3-bucket-operator rollout restart deploy/k8s-s3-bucket-operator
```

> Helm installs use a Deployment name derived from the release name; use `kubectl get deploy -n k8s-s3-bucket-operator` to find it.

### 2) Create a `BucketClass` (admin)

```yaml
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: minio-standard
driverName: k8s-s3-bucket-operator
deletionPolicy: Delete
objectLockingEnabled: true
retentionMode: GOVERNANCE
retentionDays: 30
parameters:
  region: "us-east-1"
```

Save it as `bucketclass.yaml`, then run: `kubectl apply -f bucketclass.yaml`

### 3) Create a `BucketClaim` (app team)

```yaml
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: my-app-images
  namespace: my-app
spec:
  bucketClassName: minio-standard
  protocols:
    - S3
  quota: "50Gi"
  accessType: ReadOnly
  lifecycleRules:
    - id: ExpireOldBackups
      status: Enabled
      prefix: backups/
      expiration:
        days: 30
```

Save it as `bucketclaim.yaml`, then run: `kubectl apply -f bucketclaim.yaml`

### 4) Use generated credentials

The operator creates a Secret named:

```text
<bucketclaim-name>-credentials
```

Keys in that Secret:

- `accessKeyID`
- `accessSecretKey`
- `bucketName`
- `endpoint`

### Happy path (what you should see) ✅

After a successful reconcile (names/examples may differ):

```bash
kubectl get deploy -n k8s-s3-bucket-operator
# NAME                     READY   UP-TO-DATE   AVAILABLE
# k8s-s3-bucket-operator   2/2     2            2

kubectl get bucketclass
# NAME             DRIVER                      DELETIONPOLICY   AGE
# minio-standard   k8s-s3-bucket-operator      Delete           ...

kubectl get bucketclaim -n my-app
# NAME            CLASS             ...
# my-app-images   minio-standard    ...

kubectl get secret -n my-app my-app-images-credentials
# NAME                        TYPE     DATA   AGE
# my-app-images-credentials   Opaque   4      ...
```

## Resource summary 🧩

### `BucketClaim` (`spec`)

| Field | Description |
|---|---|
| `bucketClassName` | Target `BucketClass` name |
| `bucketName` | Optional explicit bucket name |
| `protocols` | Protocol list (typically `S3`) |
| `quota` | Optional hard quota |
| `accessType` | `ReadWrite` (default) or `ReadOnly` |
| `lifecycleRules` | Optional lifecycle policy rules |
| `replicationTarget` | Optional replication target details |

### `BucketClass` (top-level fields)

| Field | Description |
|---|---|
| `driverName` | Must be `k8s-s3-bucket-operator` |
| `deletionPolicy` | `Delete` or `Retain` on claim delete |
| `objectLockingEnabled` | Enable object lock at bucket creation |
| `retentionMode` | `GOVERNANCE` or `COMPLIANCE` (declarative) |
| `retentionDays` | Retention duration in days (declarative) |
| `parameters.region` | Region used for bucket creation |
| `parameters.endpoint` | Optional per-class MinIO endpoint override |

For full semantics and verification commands (`mc quota info`, `mc ilm rule ls`), see [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md).

## For contributors only 🛠️

Developer/build commands are documented in [`CONTRIBUTING.md`](CONTRIBUTING.md).

## OpenShift (users) 🔴

OpenShift manifests live under `deploy/openshift/`. Copy/paste install (same `REPO_URL` pattern as Kubernetes):

```bash
REPO_URL="https://raw.githubusercontent.com/DevangRadadiya/k8s-s3-bucket-operator/main"
oc apply -f "${REPO_URL}/deploy/objectstorage.k8s.io_bucketclasses.yaml"
oc apply -f "${REPO_URL}/deploy/objectstorage.k8s.io_bucketclaims.yaml"
oc apply -f "${REPO_URL}/deploy/openshift/scc.yaml"
oc apply -f "${REPO_URL}/deploy/openshift/operator.yaml"
```

Then patch `minio-credentials` in namespace `k8s-s3-bucket-operator` the same way as in the Quick start.

## Supported now vs planned

### Supported now (production path)

- **Backend:** MinIO
- **API mode:** Standalone CRDs (`BucketClass`, `BucketClaim`)
- **Install methods:** Raw YAML, Kustomize, Helm OCI

### In progress / planned

- **Hybrid mode:** Standalone + COSI-compatible mode (experimental, see [`docs/COSI.md`](docs/COSI.md)).
  - Current COSI integration speaks `objectstorage.k8s.io/v1alpha1` and uses an internal GVK/JSONPath abstraction (`internal/cosi/kubecompat.go`) plus JSONPath constants in the E2E scripts so we can move to a future GA version with a focused diff.
  - COSI mode is **opt-in** and Wave 1 standalone behavior remains the default.
- **Backends:** AWS S3 first, then Ceph RGW and additional providers based on demand.
- **Advanced portability:** staged multi-backend rollout from a shared backend abstraction.

See:

- [`docs/CAPABILITY_MATRIX.md`](docs/CAPABILITY_MATRIX.md) for current capability status
- [`docs/ROADMAP.md`](docs/ROADMAP.md) for planned waves and sequence

## License

Apache License 2.0. See [`LICENSE`](LICENSE).
