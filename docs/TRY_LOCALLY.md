# Try locally (quick path)

Goal: run the operator against a local cluster and MinIO. Exact versions may vary; adapt namespaces and endpoints to your setup.

## Prerequisites

- A Kubernetes cluster ([kind](https://kind.sigs.k8s.io/), minikube, or similar)
- `kubectl` configured for that cluster
- MinIO reachable from the cluster (in-cluster MinIO or port-forward)

## 1) Install operator

**Option A — raw URLs (no clone):**

```bash
REPO_URL="https://raw.githubusercontent.com/DevangRadadiya/k8s-s3-bucket-operator/main"
kubectl apply -f "${REPO_URL}/deploy/objectstorage.k8s.io_bucketclasses.yaml"
kubectl apply -f "${REPO_URL}/deploy/objectstorage.k8s.io_bucketclaims.yaml"
kubectl apply -f "${REPO_URL}/deploy/operator.yaml"
```

**Option B — Kustomize (from a clone):**

```bash
git clone https://github.com/DevangRadadiya/k8s-s3-bucket-operator.git
cd k8s-s3-bucket-operator
kubectl apply -k deploy/
```

## 2) Point the operator at MinIO

Edit/patch the `minio-credentials` Secret in `k8s-s3-bucket-operator` so `MINIO_*` matches your MinIO Service DNS name and credentials.

```bash
kubectl -n k8s-s3-bucket-operator edit secret minio-credentials
# or use kubectl create secret ... --dry-run=client -o yaml | kubectl apply -f -
```

Restart the operator if needed:

```bash
kubectl -n k8s-s3-bucket-operator rollout restart deploy/k8s-s3-bucket-operator
```

## 3) Apply samples

Create the app namespace if needed, then:

```bash
kubectl apply -f config/samples/bucketclass.yaml
kubectl apply -f config/samples/bucketclaim.yaml
```

## 4) Automated E2E (optional)

If your environment matches what the script expects:

```bash
./test/e2e/run-e2e.sh
```

Read the script header for required namespaces and MinIO layout.

## Troubleshooting

See [`FAQ.md`](FAQ.md) and [`USER_GUIDE.md`](USER_GUIDE.md).
