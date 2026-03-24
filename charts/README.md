# Helm charts

## `k8s-s3-bucket-operator`

Install from **OCI** (after CI publishes to GHCR):

```bash
helm install k8s-s3-bucket-operator oci://ghcr.io/devangradadiya/helm-charts/k8s-s3-bucket-operator \
  --version 0.1.1 \
  --namespace k8s-s3-bucket-operator \
  --create-namespace
```

> Replace `0.1.1` with the chart version in `Chart.yaml`. Use your GitHub org/user in lowercase for `ghcr.io/...` if you fork the repo.

### From a clone (dev)

```bash
helm install k8s-s3-bucket-operator ./charts/k8s-s3-bucket-operator \
  --namespace k8s-s3-bucket-operator \
  --create-namespace
```

### Production MinIO credentials

Prefer an existing Secret:

```yaml
minio:
  existingSecret: my-minio-creds
  createSecret: false
```

Create the Secret in the operator namespace with keys `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_USE_SSL`.

### CRDs

CRDs live in `charts/k8s-s3-bucket-operator/crds/`. Helm installs them on first install; **upgrades do not automatically patch CRDs** (Helm behavior). If deploy YAML CRDs change, bump the chart and follow upgrade notes or re-apply CRDs from `deploy/`.

### Versioning (maintainers)

- **`Chart.yaml` `version`** — chart release (bump when templates/values/default image change).
- **`appVersion`** — default operator image tag when `values.image.tag` is empty.
- Keep `deploy/objectstorage.k8s.io_*.yaml` and `charts/.../crds/*.yaml` in sync when CRD schemas change.
- **`.helmignore`:** avoid `*.md` combined with `!README.md` — some Helm releases mis-handle that and report `Chart.yaml` missing. The chart ships `README.md` in the package (normal for public charts).

### Artifact Hub

See [`docs/ARTIFACT_HUB.md`](../docs/ARTIFACT_HUB.md).
