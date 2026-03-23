# Project Overview & Next Steps

This document provides a comprehensive overview of the `k8s-s3-bucket-operator` codebase and outlines the steps to take once this repository has been copied to the new server.

## Project Overview

**What we are building:**
A Kubernetes-native S3/MinIO bucket operator. Its core idea is to replace manual bucket and IAM credential creation with a declarative `kubectl apply` strategy, much like PVCs for disk storage in Kubernetes. 

**Architectural Pivot (Important Context):**
Initially, this codebase used the standard COSI (Container Object Storage Interface) official library and scaffolding, requiring up to 4 different sidecars to run alongside our driver. After evaluating our requirement constraints (specifically OpenShift-native support, multi-tenant isolation, and credential rotation), we **pivoted away from the standard COSI sidecars** while keeping the familiar CRDs (`BucketClaim`, `BucketClass`). 

This project is now a standalone, native K8s Operator built on `controller-runtime` that reconciles `BucketClaim` objects directly.

### What has been implemented (MVP):
- **API Types (`api/v1alpha1/`)**: Pure Go representation of the `BucketClaim` and `BucketClass` CRDs, including auto-generated deepcopy implementations.
- **Controller (`internal/controller/`)**: A controller-runtime Reconciler that watches `BucketClaims`. It communicates with MinIO to provision the bucket, generates standard IAM user policies, creates a K8s `Secret` scoped to the claim's namespace, and updates the claim status.
- **MinIO Interactor (`internal/minio/`)**: Reusable MinIO client that securely auto-generates AES credentials for every namespace request and strictly separates bucket access.
- **Unified Deployment & Entrypoint**: `cmd/main.go` sets up a standard K8s manager gracefully, and our deployments in `deploy/` are pared down entirely. No COSI cluster-admin generic controllers needed!

---

## Next Steps on the New Server

This `/root/cli-code` directory is completely isolated. Once you're fully cloned on the new server, pick it up from here:

### 1. Install Dependencies
Since the new server doesn't have the restrictive proxy blocking zip files, you can simply pull all Go dependencies natively:
```bash
go mod tidy
```

### 2. Verify Compilation
Ensure the K8s API schema matches controller-runtime and the Go code builds successfully:
```bash
make build
```

### 3. Test and Validation (MVP Verification)
To test the core loop of the BucketClaim controller:
1. Spin up a Kubernetes cluster (e.g., `kind` or local OpenShift).
2. Install a test instance of MinIO.
3. Deploy the operator using:
   ```bash
   make deploy
   ```
   *(Or `make deploy-openshift` if using OpenShift).*
4. Apply the sample `BucketClass` and `BucketClaim`.
5. Verify that:
   - The bucket is created in the MinIO instance.
   - The operator creates a `Secret` in the claim's namespace.
   - The `BucketClaim` status reflects `Phase: Bound` with the secret reference.
