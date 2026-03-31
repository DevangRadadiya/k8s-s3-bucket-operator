# End-to-End Tests

This folder contains the End-to-End (E2E) testing framework for the `k8s-s3-bucket-operator`.
Users and contributors can run these tests locally on a `kind` cluster to verify the operator's functionality before installing it on a production cluster.

## Running the E2E tests

The scripts automate the complete lifecycle:
1. Spins up a test MinIO instance (`minio-ns`)
2. Deploys the operator CRDs and the manager pod
3. Applies a sample `BucketClaim` and `BucketClass`
4. Polls the custom resource to ensure the `Bound` status is achieved
5. Verifies the backend S3 bucket and K8s Secret credentials were created

### Requirements
- Docker
- `kind`
- `kubectl`
- `make`
- `python3` (used by the E2E polling helpers)

### Environment tweaks

- `WAIT_TIMEOUT` — default `120s`; used for most `kubectl rollout status` waits.
- `COSI_WAIT_TIMEOUT` — default `300s`; used only after COSI wiring (operator + `objectstorage-controller`), since extra containers/images may need longer pulls.
- `KEEP_E2E_ARTIFACTS=1` — skip namespace/CRD cleanup on exit (debugging).

### Execute (Kubernetes profile)

From the project root, run:
```bash
./test/e2e/run-e2e.sh
```

### Execute (OpenShift profile)

```bash
./test/e2e/run-e2e-openshift.sh
```

### Unified entrypoint

```bash
./test/e2e/run.sh k8s
./test/e2e/run.sh openshift
```

### Dev workflow reminder (for contributors)

For PRs, create a dev/feature branch from `main` (example: `git checkout -b dev/my-wave1-fix`), run unit tests (`go test ./...`) and E2E (`./test/e2e/run.sh k8s`), then open a PR against `main`.

### Test custom operator image

Use this to validate a just-built image before release:

```bash
OPERATOR_IMAGE=ghcr.io/devangradadiya/k8s-s3-bucket-operator:main ./test/e2e/run.sh k8s
OPERATOR_IMAGE=ghcr.io/devangradadiya/k8s-s3-bucket-operator:main ./test/e2e/run.sh openshift
```

### Test with a local operator image (recommended)

If you want the E2E to test the code from your current working tree, build a local image and load it into your `kind` cluster:

```bash
make docker-build IMG=k8s-s3-bucket-operator:test

# Load into every kind cluster you have (default name is often chart-testing)
for c in $(kind get clusters); do
  kind load docker-image k8s-s3-bucket-operator:test --name "$c"
done

# Run E2E using the same local tag
OPERATOR_IMAGE=k8s-s3-bucket-operator:test ./test/e2e/run.sh k8s
```

When the image is only on the node (e.g. **kind load docker-image**), the script sets **imagePullPolicy** to `IfNotPresent` after `kubectl set image` so Kubernetes does not try to pull a non-registry tag with **Always**.

**kind:** the default cluster name from [helm/kind-action](https://github.com/helm/kind-action) is often `chart-testing`, not `kind`. Load with:

`kind load docker-image <image:tag> --name chart-testing`

(or loop `kind get clusters` as in `.github/workflows/test.yml`).

You can also set `KUBECTL=oc` for OpenShift CLI environments.
