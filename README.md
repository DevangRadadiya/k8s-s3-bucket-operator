# k8s-s3-bucket-operator
Kubernetes-native S3 bucket provisioning and access management for MinIO, AWS S3, and Ceph. OpenShift-ready. COSI-based.

**Kubernetes-native S3 bucket provisioning and access management.**
Automatically create and manage S3-compatible buckets (MinIO, AWS S3, Ceph) directly from Kubernetes — no manual IAM setup, no external scripts.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.25%2B-326CE5?logo=kubernetes)](https://kubernetes.io)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://go.dev)
[![OpenShift Ready](https://img.shields.io/badge/OpenShift-Ready-EE0000?logo=redhatopenshift)](https://www.redhat.com/en/technologies/cloud-computing/openshift)
[![COSI](https://img.shields.io/badge/COSI-based-orange)](https://container-object-storage-interface.github.io)

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

## Features

- **Automatic bucket provisioning** — declare a `BucketClaim`, get a bucket
- **Secure credential management** — credentials stored in Kubernetes Secrets, never logged
- **Namespace isolation** — each namespace gets its own scoped access (multi-tenancy)
- **Bucket lifecycle policies** — expiry, archival, cleanup rules via CRD
- **OpenShift-ready** — runs non-root, SCC-compatible, uses Routes
- **Multi-backend** — MinIO, AWS S3, Ceph RGW (any S3-compatible store)
- **Credential rotation** — rotate access keys without app downtime

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

### 1. Install the COSI controller

```bash
kubectl apply -f https://raw.githubusercontent.com/DevangRadadiya/k8s-s3-bucket-operator/main/deploy/cosi-controller.yaml
```

### 2. Install the operator

```bash
kubectl apply -f https://raw.githubusercontent.com/DevangRadadiya/k8s-s3-bucket-operator/main/deploy/operator.yaml
```

### 3. Create a BucketClass (admin, once per backend)

```yaml
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: minio-standard
driverName: k8s-s3-bucket-operator
deletionPolicy: Delete
parameters:
  endpoint: "http://minio.minio-ns.svc.cluster.local:9000"
  region: "us-east-1"
```

```bash
kubectl apply -f bucketclass.yaml
```

### 4. Claim a bucket (developer, per app)

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

### 5. Access credentials in your app

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

### BucketClaim

| Field | Description | Required |
|---|---|---|
| `bucketClassName` | Which BucketClass to use | Yes |
| `protocols` | Storage protocols (`S3`) | Yes |
| `bucketName` | Explicit bucket name (auto-generated if omitted) | No |

### BucketClass Parameters

| Parameter | Description | Default |
|---|---|---|
| `endpoint` | S3/MinIO endpoint URL | — |
| `region` | S3 region | `us-east-1` |
| `deletionPolicy` | `Delete` or `Retain` on claim deletion | `Retain` |

---

## Roadmap

- [x] MinIO bucket provisioning
- [x] Automatic credential generation
- [x] Namespace isolation
- [x] OpenShift SCC support
- [ ] Bucket lifecycle policies (expiry, archival)
- [ ] Credential rotation
- [ ] AWS S3 backend
- [ ] Ceph RGW backend
- [ ] Prometheus metrics
- [ ] Helm chart
- [ ] Bucket quota management

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
