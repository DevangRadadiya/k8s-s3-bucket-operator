# k8s-s3-bucket-operator (Helm chart)

Deploys the **k8s-s3-bucket-operator** (CRDs + RBAC + Deployment + MinIO Secret).

## Install (OCI)

```bash
helm install k8s-s3-bucket-operator oci://ghcr.io/devangradadiya/helm-charts/k8s-s3-bucket-operator \
  --version 0.1.1 \
  --namespace k8s-s3-bucket-operator \
  --create-namespace
```

## Important values

| Key | Description |
|-----|-------------|
| `minio.existingSecret` | Use a pre-created Secret (recommended for prod) |
| `minio.createSecret` | Create Secret from `minio.*` values (default `true` for quick start) |
| `image.repository` / `image.tag` | Operator image |
| `replicaCount` | Deployment replicas (leader election enabled by default) |

See `values.yaml` for the full list.

## Documentation

- [User guide](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/blob/main/docs/USER_GUIDE.md)
- [Parent charts README](../README.md)
