# Monitoring (Prometheus)

The operator exposes **Prometheus metrics** on **`/metrics`** at **`0.0.0.0:8080`** (controller-runtime default), separate from health probes on **`:8081`**.

## Metric names

| Metric | Labels | Meaning |
|--------|--------|---------|
| `bucket_operator_reconcile_total` | `result` | Reconcile outcomes (`success`, `error`, `deleted`, …) |
| `bucket_operator_reconcile_errors_total` | `stage` | Errors by stage (`create_bucket`, `grant_access`, …) |
| `bucket_operator_reconcile_duration_seconds` | `bucketclass` | Reconcile latency histogram |
| `bucket_operator_buckets_bound_total` | `bucketclass` | Successful binds (counter) |

Standard **Go runtime** and **workqueue** metrics from controller-runtime are also available on the same endpoint.

## Raw manifests (`deploy/operator.yaml`)

After `kubectl apply`, scrape the operator Pods or the metrics Service:

- **Service:** `k8s-s3-bucket-operator-metrics` in namespace `k8s-s3-bucket-operator` (port **8080**).

Example **ServiceMonitor** (Prometheus Operator — adjust `release` / `namespace` to match your install):

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: k8s-s3-bucket-operator
  namespace: k8s-s3-bucket-operator
spec:
  selector:
    matchLabels:
      app: k8s-s3-bucket-operator
  namespaceSelector:
    matchNames:
      - k8s-s3-bucket-operator
  endpoints:
    - port: metrics
      path: /metrics
      interval: 30s
```

## Helm

The chart can create a ClusterIP **metrics** Service when `metrics.service.enabled` is `true` (default). Point your `ServiceMonitor` at that Service’s port.

## Grafana

Import [`deploy/grafana-dashboard.json`](../deploy/grafana-dashboard.json) and select your Prometheus datasource.

## OpenShift

The OpenShift manifest set under `deploy/openshift/` includes the same metrics **Service** and **PodDisruptionBudget** patterns as the Kubernetes `deploy/operator.yaml` path.
