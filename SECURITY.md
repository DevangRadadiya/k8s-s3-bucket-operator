# Security policy

## Supported versions

Security updates are applied to the **default branch** (`main`) and published via container images (e.g. `ghcr.io/devangradadiya/k8s-s3-bucket-operator:latest` and CI tags). Use tagged releases when available for the most predictable upgrades.

## Reporting a vulnerability

**Please do not open a public GitHub issue** for security vulnerabilities.

Instead:

1. Open a **[private security advisory](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/security/advisories/new)** on this repository (if you have access), **or**
2. Email the maintainers with a clear subject line, e.g. `[SECURITY] k8s-s3-bucket-operator`, describing:
   - Affected component (operator, CRDs, MinIO client behavior, etc.)
   - Steps to reproduce
   - Impact assessment if known

We aim to acknowledge reports within a few business days and coordinate a fix and disclosure timeline.

## Scope

In scope: the operator binary, Kubernetes manifests in this repo, and documented usage patterns.

Out of scope: vulnerabilities in upstream dependencies (report to MinIO, Kubernetes, etc.) unless this project uses them in an unsafe way that we can fix here.

## Safe defaults

- Run the operator with least-privilege RBAC.
- Protect MinIO admin credentials used by the operator; treat generated bucket credentials as secrets.
- Keep cluster, operator image, and MinIO versions patched.
