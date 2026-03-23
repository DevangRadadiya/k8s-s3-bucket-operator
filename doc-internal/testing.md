# Operator End-to-End Testing Reference

This document provides a reference on how to test the completely re-architected `k8s-s3-bucket-operator`. We have moved away from the complex external COSI sidecars, and natively implemented `BucketClaim` and `BucketClass` controllers.

## Prerequisites
- A running Kubernetes cluster (e.g., `kind create cluster`)
- `kubectl` configured
- The statically compiled `bin/manager` binary and its corresponding Docker image `ghcr.io/devangradadiya/k8s-s3-bucket-operator:latest` loaded to the cluster.

## Deployment Steps
1. **MinIO Test Environment:** Deploy a MinIO test instance into the `minio-ns` namespace using the manifest provided in `config/samples/minio.yaml`. The operator accesses this backend natively via the `minioadmin:minioadmin` credentials.
2. **Operator Installation:** Use `make deploy` to apply the CRDs and deploy the operator.
3. **Application Verification:** Apply the samples in `config/samples/bucketclass.yaml` and `config/samples/bucketclaim.yaml` to request a bucket in the `my-app` namespace.

## Verification
You can verify the operator is functioning correctly exactly 3 ways:

1. **Claim Status:** `kubectl get bucketclaim my-app-images -n my-app -o yaml` -> the `phase` will transition to `Bound`.
2. **Secret Generation:** The operator automatically generates standard S3 credentials back into the application's namespace. Verify using `kubectl get secret my-app-images-credentials -n my-app`.
3. **Storage Verification:** Verify the bucket directory physically exists on the MinIO backend by exec-ing into the container: `kubectl exec -n minio-ns deploy/minio -- ls -l /data/`.

## Automated Testing
An automated test script `test.sh` is provided in the repository root. It executes these steps, polls the API for the `Bound` status, and validates the Secret and backend storage.
