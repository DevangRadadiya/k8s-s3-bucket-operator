# Log archival bucket

Longer retention for compressed logs. Adjust `expiration.days` to your compliance window.

```bash
kubectl apply -f bucketclass.yaml
kubectl apply -f bucketclaim.yaml
```
