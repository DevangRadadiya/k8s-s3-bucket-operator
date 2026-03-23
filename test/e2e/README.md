# End-to-End Tests

This folder contains the End-to-End (E2E) testing framework for the `k8s-s3-bucket-operator`.
Users and contributors can run these tests locally on a `kind` cluster to verify the operator's functionality before installing it on a production cluster.

## Running the E2E Test

The `run-e2e.sh` script automates the complete lifecycle:
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

### Execute

From the project root, run:
```bash
./test/e2e/run-e2e.sh
```
