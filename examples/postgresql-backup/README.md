# PostgreSQL backup bucket

Creates a bucket with quota and lifecycle so old backups expire under a prefix.

1. Edit `bucketclaim.yaml` namespace if needed.
2. Apply:

```bash
kubectl apply -f bucketclass.yaml
kubectl apply -f bucketclaim.yaml
```

3. Use the generated Secret `<claim-name>-credentials` in your backup Job (endpoint, keys, bucket name).
