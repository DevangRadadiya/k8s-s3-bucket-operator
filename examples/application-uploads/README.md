# Application uploads bucket

Typical **ReadWrite** bucket for user or service uploads. Tighten `accessType` to **ReadOnly** on the workload if uploads go through a separate pipeline.

```bash
kubectl apply -f bucketclass.yaml
kubectl apply -f bucketclaim.yaml
```
