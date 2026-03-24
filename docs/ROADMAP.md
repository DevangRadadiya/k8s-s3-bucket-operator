# Public Roadmap

This roadmap reflects planned direction and sequencing.  
It is intentionally staged to keep reliability high while expanding scope.

## Product direction

- Keep MinIO-first path stable and production-friendly.
- Add hybrid API support (standalone + COSI) without breaking current users.
- Expand to multi-backend one provider at a time.
- Add cross-bucket and cross-backend workflows only after core maturity.

## Current status (today)

### Supported now

- MinIO backend
- Standalone CRDs (`BucketClass`, `BucketClaim`)
- Install via raw YAML, Kustomize, and Helm OCI

### Planned

- COSI-compatible mode
- AWS S3 backend (first multi-backend target)
- Ceph RGW and additional providers after AWS maturity

## Delivery waves

### Wave 1: Reliability and adoption (now to ~8 weeks)

- Improve observability (metrics and dashboard)
- Strengthen CI quality gates and test confidence
- Expand real-world examples and operator runbooks
- Keep README and docs copy-paste friendly

### Wave 2: Hybrid API mode (target ~3-6 months)

- Add COSI-compatible mode while preserving standalone mode
- Reuse shared backend/business logic
- Publish migration guidance and known limitations

### Wave 3: Multi-backend foundation (target ~6-12 months)

- Introduce backend abstraction layer
- Keep MinIO as baseline adapter
- Add AWS backend first with end-to-end validation
- Publish capability matrix per backend

### Wave 4: Cross-bucket and cross-backend workflows (post-foundation)

- Cross-bucket policy patterns
- Controlled cross-backend replication/migration workflows
- Strong guardrails, auditability, and runbooks

## Support policy

A backend or mode is marked as "supported" only when all of the following exist:

1. Automated tests (including E2E)
2. User documentation and examples
3. Capability matrix entries
4. Operational troubleshooting guidance

Until then, it remains "planned" or "experimental."

