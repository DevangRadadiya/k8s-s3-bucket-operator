# FAQ

Quick answers for common questions. For full detail see [`USER_GUIDE.md`](USER_GUIDE.md).

## Install & setup

### Do I need to build the operator from source?

**No.** Use the published container image and YAML from this repo. See the [README](../README.md) Quick start or run `kubectl apply -k deploy/` from a clone.

### Where do I set MinIO connection details?

In the `minio-credentials` Secret in namespace `k8s-s3-bucket-operator` (created by `deploy/operator.yaml`). Set `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, and `MINIO_USE_SSL`, then restart the operator Deployment if it already started with wrong values.

### Which image tag should I use?

Default manifests use `ghcr.io/devangradadiya/k8s-s3-bucket-operator:latest`. CI also publishes `:main` on every push to `main`, and semver tags on GitHub Releases.

---

## BucketClaim / BucketClass

### My `BucketClaim` never becomes ready / no Secret

1. `kubectl describe bucketclaim -n <ns> <name>` — check events and status.
2. `kubectl logs -n k8s-s3-bucket-operator deploy/k8s-s3-bucket-operator` — look for reconcile errors (often wrong MinIO endpoint or credentials).
3. Confirm `BucketClass` exists and `driverName` is `k8s-s3-bucket-operator`.

### Can I enable object lock on an existing bucket?

**No.** Object locking must be set when the bucket is created. Use `BucketClass.objectLockingEnabled` for new buckets only.

### Quota or lifecycle does not show up immediately in `mc`

Quota enforcement can depend on MinIO background scanning. For lifecycle, use `mc ilm rule ls` (not deprecated `mc ilm ls` on newer clients). Confirm the operator image matches the CRD features you expect.

### Does replication “just work”?

**Replication is advanced.** The operator can configure a replication rule toward a target, but the target cluster must be reachable, credentials must be valid, and MinIO may need extra setup. Validate in your environment.

---

## Security

### How do I report a security issue?

Do **not** open a public issue. Follow [`SECURITY.md`](../SECURITY.md) (private advisory or maintainer contact).

---

## Getting help

- **Bug reports:** [GitHub Issues](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/issues) (use the bug template).
- **Questions:** [GitHub Discussions](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/discussions) if enabled on the repo; otherwise open a discussion-style issue.
