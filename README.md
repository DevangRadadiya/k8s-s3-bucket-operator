# k8s-s3-bucket-operator

Kubernetes operator for provisioning and managing S3-compatible buckets (MinIO first) through `BucketClass` and `BucketClaim` resources.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.25%2B-326CE5?logo=kubernetes)](https://kubernetes.io)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev)
[![OpenShift Ready](https://img.shields.io/badge/OpenShift-Ready-EE0000?logo=redhatopenshift)](https://www.redhat.com/en/technologies/cloud-computing/openshift)

## What this operator does

- Reconciles `BucketClaim` objects and creates buckets in MinIO.
- Generates per-claim credentials and stores them in namespaced Kubernetes Secrets.
- Applies claim-level controls like quota, lifecycle, access type, and replication target.
- Enforces class-level settings like deletion policy and object lock at bucket creation.

## Key features

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

## Documentation

| Doc | Location | Purpose |
|---|---|---|
| User guide | [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md) | Full install + CRD behavior + troubleshooting |
| Release checklist | [`docs/RELEASE_CHECKLIST.md`](docs/RELEASE_CHECKLIST.md) | Tag/release process and verification |
| Contributing | [`CONTRIBUTING.md`](CONTRIBUTING.md) | Dev workflow and PR guidelines |
| Changelog | [`CHANGELOG.md`](CHANGELOG.md) | User-visible project changes |
| Security policy | [`SECURITY.md`](SECURITY.md) | Private vulnerability reporting process |
| Code of conduct | [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) | Contributor Covenant 3.0 |

## Quick start

### Prerequisites

- Kubernetes 1.25+ (or OpenShift 4.12+)
- A reachable MinIO endpoint
- `kubectl` (or `oc`)

### 1) Deploy CRDs and operator

From repo root:

```bash
make deploy
```

Equivalent raw apply:

```bash
kubectl apply -f deploy/objectstorage.k8s.io_bucketclasses.yaml
kubectl apply -f deploy/objectstorage.k8s.io_bucketclaims.yaml
kubectl apply -f deploy/operator.yaml
```

Before deploying, set valid MinIO credentials in `deploy/operator.yaml` (`Secret` named `minio-credentials`):

- `MINIO_ENDPOINT`
- `MINIO_ACCESS_KEY`
- `MINIO_SECRET_KEY`
- `MINIO_USE_SSL`

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

```bash
kubectl apply -f config/samples/bucketclass.yaml
```

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

```bash
kubectl apply -f config/samples/bucketclaim.yaml
```

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

## Resource summary

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

## Operations

Useful Make targets:

```bash
make build
make test
make deploy
make undeploy
make samples
make samples-clean
make deploy-openshift
```

## OpenShift

OpenShift manifests are available in `deploy/openshift/` and include SCC-friendly settings.

```bash
oc apply -f deploy/objectstorage.k8s.io_bucketclasses.yaml
oc apply -f deploy/objectstorage.k8s.io_bucketclaims.yaml
oc apply -f deploy/openshift/scc.yaml
oc apply -f deploy/openshift/operator.yaml
```

## Current backend support

- MinIO: supported
- AWS S3: planned
- Ceph RGW: planned

## License

Apache License 2.0. See [`LICENSE`](LICENSE).
# k8s-s3-bucket-operator

**Kubernetes-native S3 bucket provisioning and access management.**
Automatically create and manage S3-compatible buckets (MinIO, AWS S3, Ceph) directly from Kubernetes — no manual IAM setup, no external scripts.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.25%2B-326CE5?logo=kubernetes)](https://kubernetes.io)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev)
[![OpenShift Ready](https://img.shields.io/badge/OpenShift-Ready-EE0000?logo=redhatopenshift)](https://www.redhat.com/en/technologies/cloud-computing/openshift)
[![COSI](https://img.shields.io/badge/COSI-based-orange)](https://container-object-storage-interface.github.io)

---

## Documentation

| Doc | Location | Description |
|-----|----------|-------------|
| **User guide** | [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md) | Install, CRD fields, enterprise features, `mc` checks, troubleshooting |
| **Samples** | [`config/samples/`](config/samples/) | `bucketclass.yaml`, `bucketclaim.yaml` with optional enterprise fields |
| **Changelog** | [`CHANGELOG.md`](CHANGELOG.md) | Release notes (Keep a Changelog); update when you tag releases |
| **Security** | [`SECURITY.md`](SECURITY.md) | How to report vulnerabilities (private advisories) |
| **Code of conduct** | [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) | Contributor Covenant |
| **Contributing** | [`CONTRIBUTING.md`](CONTRIBUTING.md) | Build, test, PR workflow |
| **Release checklist** | [`docs/RELEASE_CHECKLIST.md`](docs/RELEASE_CHECKLIST.md) | Step-by-step release/tag process and post-release verification |

**Where things live:** **Product docs** stay under **`docs/`**. **Community / GitHub conventions** (`CHANGELOG`, `SECURITY`, `CODE_OF_CONDUCT`) stay at the **repo root** so GitHub and tooling pick them up automatically. **Issue templates** and the **PR template** live under **`.github/`**.

**README vs wiki:** The user guide in **`docs/`** is versioned with the code. You can optionally mirror into [GitHub Wiki](https://docs.github.com/en/communities/documenting-your-project-with-wikis/about-wikis); treat **`docs/` as the source of truth**.

---

## Why this exists

Kubernetes handles disk storage well via CSI and PersistentVolumes.
But **object storage (S3 buckets)** is different — it's API-based, not mountable — and until now it required manual setup outside Kubernetes.

This operator brings bucket provisioning **into** Kubernetes using the [COSI](https://container-object-storage-interface.github.io) standard.

**Before this operator:**
```
Developer → DevOps → Create bucket manually → Create IAM user manually
         → Inject credentials manually → Hope nothing drifts
```

**With this operator:**
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
```
```
kubectl apply -f bucketclaim.yaml
→ Bucket created automatically
→ Credentials generated and stored in a Secret
→ App gets endpoint + keys, ready to use
```

---

### MinIO Compatibility
This operator natively imports `github.com/minio/minio-go/v7`. Because it leverages standard AWS S3 v4 signatures and the newest MinIO Admin APIs, it inherently supports the latest MinIO features (including Object Locking, PBAC, and KMS encryption targets) seamlessly. No dependencies need to be bumped right now to support modern MinIO releases.

## Market Comparison

There are several ways to provision S3 buckets from Kubernetes. Here is how we compare:

| Feature / Operator | This Repo (`k8s-s3-bucket-operator`) | Official COSI Ecosystem | Crossplane (Provider AWS/S3) | Rook / Ceph |
|---|---|---|---|---|
| **Architecture** | **Zero-sidecar**, single lightweight deployment | **Heavy**: requires 4 distinct cluster-admin sidecars | **Heavy**: massive CRD footprint for a single bucket | **Coupled**: strongly tied to Ceph architectures |
| **Auto-Secrets** | Localized Secrets injected into app namespace | Supported but complex | Supported | Supported |
| **OpenShift Ready**| ✅ Native (`restricted-v2` SCC compliant) | ❌ Requires custom privileges | ❌ Often requires custom privileges | ❌ Can struggle with non-root |
| **Complexity** | **Very Low** | **Very High** (complex troubleshooting) | **High** | **High** |

---

## Features

- **Automatic bucket provisioning** — declare a `BucketClaim`, get a bucket
- **Secure credential management** — credentials stored in Kubernetes Secrets, never logged
- **Namespace isolation** — each namespace gets its own scoped access (multi-tenancy)
- **Bucket quotas** — optional hard quota per claim (`spec.quota`, e.g. `50Gi`)
- **Lifecycle rules** — prefix + expiration days via `spec.lifecycleRules`
- **Object lock (bucket creation)** — optional on `BucketClass` (`objectLockingEnabled`, retention fields)
- **IAM-style access** — `ReadWrite` or `ReadOnly` per claim (`spec.accessType`)
- **Replication target** — optional rule toward another MinIO endpoint (`spec.replicationTarget`; advanced)
- **Deletion policy** — `Delete` or `Retain` bucket on claim deletion (`BucketClass.deletionPolicy`)
- **OpenShift-ready** — runs non-root, SCC-compatible, uses Routes
- **Multi-backend** — MinIO today; AWS S3 / Ceph RGW planned

---

## Supported Backends

| Backend | Status |
|---|---|
| MinIO | ✅ Supported |
| AWS S3 | 🔜 Planned |
| Ceph RGW | 🔜 Planned |
| Wasabi | 🔜 Planned |

---

## How it works

```
┌─────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                   │
│                                                         │
│  Developer                                              │
│     │                                                   │
│     │  kubectl apply BucketClaim                        │
│     ▼                                                   │
│  ┌──────────────────────┐                               │
│  │  k8s-s3-bucket-      │   1. Watch BucketClaim        │
│  │  operator            │   2. Create bucket in MinIO   │
│  │  (COSI driver)       │   3. Generate credentials     │
│  │                      │   4. Store in Secret          │
│  └──────────┬───────────┘   5. Update BucketClaim status│
│             │                                           │
│             ▼                                           │
│  ┌──────────────────────┐                               │
│  │  Kubernetes Secret   │  ACCESS_KEY, SECRET_KEY,      │
│  │  (your namespace)    │  BUCKET_NAME, ENDPOINT        │
│  └──────────────────────┘                               │
│             │                                           │
│             ▼                                           │
│  ┌──────────────────────┐                               │
│  │  Your App Pod        │  Uses SDK/API to access       │
│  │                      │  bucket — no manual setup     │
│  └──────────────────────┘                               │
└─────────────────────────────────────────────────────────┘
                    │
                    ▼
         ┌──────────────────┐
         │   MinIO / S3     │
         │   (external or   │
         │   in-cluster)    │
         └──────────────────┘
```

---

## Quick Start

### Prerequisites

- Kubernetes 1.25+ or OpenShift 4.12+
- MinIO instance (in-cluster or external)
- kubectl / oc

### 1. Install CRDs and operator

From a clone:

```bash
make deploy
```

Or with raw URLs (same as `make deploy`):

```bash
kubectl apply -f https://raw.githubusercontent.com/DevangRadadiya/k8s-s3-bucket-operator/main/deploy/objectstorage.k8s.io_bucketclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/DevangRadadiya/k8s-s3-bucket-operator/main/deploy/objectstorage.k8s.io_bucketclaims.yaml
kubectl apply -f https://raw.githubusercontent.com/DevangRadadiya/k8s-s3-bucket-operator/main/deploy/operator.yaml
```

Edit the **`minio-credentials`** Secret in `deploy/operator.yaml` (or patch after apply) so `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, and `MINIO_SECRET_KEY` point at your MinIO.

> **Note:** This project is COSI-aligned in spirit; a separate COSI controller manifest is not required for the current `BucketClaim` / `BucketClass` flow. See the [user guide](docs/USER_GUIDE.md) for details.

### 2. Create a BucketClass (admin, once per backend)

```yaml
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: minio-standard
driverName: k8s-s3-bucket-operator
deletionPolicy: Delete
parameters:
  region: "us-east-1"
  # Optional: endpoint override per class (else operator uses MINIO_ENDPOINT)
  # endpoint: "http://minio.minio-ns.svc.cluster.local:9000"
```

```bash
kubectl apply -f bucketclass.yaml
```

### 3. Claim a bucket (developer, per app)

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
```

```bash
kubectl apply -f bucketclaim.yaml
```

For **quotas, lifecycle, read-only access, replication**, see [`config/samples/bucketclaim.yaml`](config/samples/bucketclaim.yaml) and the [user guide](docs/USER_GUIDE.md).

### 4. Access credentials in your app

```yaml
env:
  - name: AWS_ACCESS_KEY_ID
    valueFrom:
      secretKeyRef:
        name: my-app-images-credentials
        key: accessKeyID
  - name: AWS_SECRET_ACCESS_KEY
    valueFrom:
      secretKeyRef:
        name: my-app-images-credentials
        key: accessSecretKey
  - name: BUCKET_NAME
    valueFrom:
      secretKeyRef:
        name: my-app-images-credentials
        key: bucketName
  - name: BUCKET_ENDPOINT
    valueFrom:
      secretKeyRef:
        name: my-app-images-credentials
        key: endpoint
```

---

## OpenShift Installation

This operator is fully compatible with OpenShift. It runs non-root and respects SCCs.

```bash
# Create namespace
oc new-project k8s-s3-bucket-operator

# Apply SCC
oc apply -f deploy/openshift/scc.yaml

# Deploy
oc apply -f deploy/openshift/operator.yaml
```

> Routes are used instead of LoadBalancer services for OpenShift environments.

---

## Use Cases

| Use Case | Example |
|---|---|
| App image/video storage | Store user uploads from a web app |
| Backup & disaster recovery | Velero backup target |
| Log archival | Fluentd / Loki log sink |
| AI/ML datasets | Training data for Kubeflow pipelines |
| Data pipelines | Spark / Flink intermediate storage |
| Multi-tenant SaaS | One bucket per namespace/tenant |

---

## Configuration Reference

Full tables and behavior notes: **[docs/USER_GUIDE.md](docs/USER_GUIDE.md)**.

### BucketClaim (`spec`)

| Field | Description | Required |
|---|---|---|
| `bucketClassName` | Which BucketClass to use | Yes |
| `protocols` | Storage protocols (`S3`) | Recommended |
| `bucketName` | Explicit bucket name (default `<namespace>-<claim-name>`) | No |
| `quota` | Hard quota (e.g. `50Gi`) | No |
| `accessType` | `ReadWrite` or `ReadOnly` for generated credentials | No (default `ReadWrite`) |
| `lifecycleRules` | Prefix + `expiration.days` rules | No |
| `replicationTarget` | Remote endpoint, bucket, keys for replication rule | No |

### BucketClass (top-level fields)

| Field | Description | Default |
|---|---|---|
| `driverName` | Must be `k8s-s3-bucket-operator` | — |
| `deletionPolicy` | `Delete` or `Retain` bucket when claim is deleted | `Retain` |
| `objectLockingEnabled` | Enable object lock at **bucket creation** | `false` |
| `retentionMode` | `GOVERNANCE` or `COMPLIANCE` | — |
| `retentionDays` | Retention duration in days (declarative / compliance alignment) | — |
| `parameters.region` | S3 region for bucket creation | `us-east-1` if empty |
| `parameters.endpoint` | Override MinIO endpoint for this class | Operator `MINIO_ENDPOINT` if omitted |

---

## Roadmap

- [x] MinIO bucket provisioning
- [x] Automatic credential generation
- [x] Namespace isolation
- [x] OpenShift SCC support
- [x] Bucket lifecycle rules (prefix + expiration days)
- [x] Bucket quota (hard quota via MinIO Admin API)
- [x] Object lock at bucket creation + retention fields on class
- [x] Read-only vs read-write generated policies
- [x] Replication target (advanced; environment-dependent)
- [ ] Credential rotation
- [ ] AWS S3 backend
- [ ] Ceph RGW backend
- [ ] Prometheus metrics
- [ ] Helm chart

---

## Architecture

Built on:
- [COSI](https://container-object-storage-interface.github.io) — Kubernetes SIG standard for object storage
- [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) — Kubernetes controller framework
- [MinIO Go SDK](https://github.com/minio/minio-go) — MinIO/S3 client

---

## Contributing

Contributions welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

Areas most needed right now:
- AWS S3 backend implementation
- Ceph RGW backend
- Helm chart
- More end-to-end tests

---

## License

Apache License 2.0 — see [LICENSE](LICENSE).

---

## Related Projects

- [kubernetes-sigs/container-object-storage-interface](https://github.com/kubernetes-sigs/container-object-storage-interface) — COSI spec
- [IBM/s3-iam-cosi-driver](https://github.com/IBM/s3-iam-cosi-driver) — IBM's COSI implementation
- [scality/cosi-driver](https://github.com/scality/cosi-driver) — Scality's COSI driver
- [InseeFrLab/s3-operator](https://github.com/InseeFrLab/s3-operator) — S3 operator (non-COSI)
