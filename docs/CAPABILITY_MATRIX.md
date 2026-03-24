# Capability Matrix

This matrix shows current support status by mode/backend.

Status meaning:

- `Supported`: implemented and documented as current path
- `Planned`: on roadmap, not available yet
- `Partial`: available with constraints or pending hardening

## Modes and backends

| Area | Standalone + MinIO | COSI mode + MinIO | Standalone + AWS | Standalone + Ceph RGW |
|---|---|---|---|---|
| Availability | Supported | Planned | Planned | Planned |
| Bucket provisioning | Supported | Planned | Planned | Planned |
| Credential secret generation | Supported | Planned | Planned | Planned |
| Quota management | Supported | Planned | Planned | Planned |
| Lifecycle rules | Supported | Planned | Planned | Planned |
| Access type (`ReadOnly`/`ReadWrite`) | Supported | Planned | Planned | Planned |
| Object lock toggle | Supported | Planned | Planned | Planned |
| Replication target field | Partial | Planned | Planned | Planned |
| Deletion policy (`Delete`/`Retain`) | Supported | Planned | Planned | Planned |
| OpenShift manifests | Supported | Planned | Planned | Planned |

## Notes

- MinIO standalone mode is the current production path.
- COSI mode is planned to be introduced in hybrid fashion (no break for current users).
- AWS is the first planned backend after hybrid baseline.
- Additional providers are staged after core multi-backend maturity.

