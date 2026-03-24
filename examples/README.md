# Examples

Copy-paste **BucketClass** + **BucketClaim** patterns for common production-style use cases.

**Prerequisites:** operator installed, MinIO reachable, and a `BucketClass` driver name of `k8s-s3-bucket-operator`.

| Example | Purpose |
|---------|---------|
| [postgresql-backup](postgresql-backup/) | Bucket + lifecycle for database dumps |
| [application-uploads](application-uploads/) | Read/write bucket for app binary uploads |
| [logs-archival](logs-archival/) | Long-retention bucket for log archives |

Each folder is self-contained: apply `bucketclass.yaml` then `bucketclaim.yaml` (adjust namespaces and names).
