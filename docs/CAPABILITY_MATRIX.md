# Capability Matrix

This matrix shows current support status by mode/backend.

Status meaning:

- `Supported`: implemented and documented as current path
- `Planned`: on roadmap, not available yet
- `Partial`: available with constraints or pending hardening

## Modes and backends

| Area | Standalone + MinIO | COSI mode + MinIO | Standalone + AWS | Standalone + Ceph RGW |
|---|---|---|---|---|
| Availability | Supported | Supported | Supported (MVP) | Planned |
| Bucket provisioning | Supported | Supported | Supported | Planned |
| Credential secret generation | Supported | Supported | Supported | Planned |
| Quota management | Supported | Supported | Partial (no-op) | Planned |
| Lifecycle rules | Supported | Supported | Supported | Planned |
| Access type (`ReadOnly`/`ReadWrite`) | Supported | Supported | Supported | Planned |
| Object lock toggle | Supported | Supported | Supported (toggle only) | Planned |
| Replication target field | Partial | Partial | Partial (no-op) | Planned |
| Deletion policy (`Delete`/`Retain`) | Supported | Supported | Supported | Planned |
| Bucket security hardening (public access block, ACL disable, encryption, TLS-only policy) | Partial | Partial | Supported | Planned |
| Custom S3 bucket policy attachment (`bucketPolicyRef`) | Planned | Planned | Supported | Planned |
| OpenShift manifests | Supported | Supported | Supported | Planned |

## Notes

- MinIO standalone mode is the current production path.
- COSI mode is available and shares the same provisioning backend interface.
- AWS support is currently **Standalone (MVP)** via `BucketClass.backend: AWS` and `awsCredentialSecretRef`.
- AWS limitations (current): bucket quota is a no-op; replication config is a no-op.
- Additional providers are staged after core multi-backend maturity.

