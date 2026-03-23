## Summary

- Describe the user-facing change in 1-3 bullets.

## Why

- What problem does this PR solve?

## Changes

- [ ] API / CRD updates
- [ ] Controller / reconcile logic
- [ ] MinIO client / backend behavior
- [ ] Docs / samples
- [ ] CI / release flow

## Test Plan

- [ ] `go test ./...`
- [ ] E2E smoke path (claim -> secret -> bucket)
- [ ] Negative/edge case tested (describe below)

## Validation Details

Include exact commands and key outputs (sanitized):

```bash
# example
kubectl get bucketclaims -A
kubectl logs -n k8s-s3-bucket-operator deploy/k8s-s3-bucket-operator
```

## Breaking Changes

- [ ] No breaking changes
- [ ] Yes (describe migration/compat notes)

## Security and Operations

- [ ] No new credentials/secrets exposure
- [ ] RBAC impact reviewed
- [ ] Image tag/deploy manifest impact reviewed

## Checklist

- [ ] I updated docs (`README.md` and/or `docs/*`) where needed.
- [ ] I added/updated tests where practical.
- [ ] I confirmed no sensitive data is committed.
