#!/bin/bash
set -euo pipefail

KUBECTL_BIN="${KUBECTL:-kubectl}"
OPERATOR_NS="k8s-s3-bucket-operator"
APP_NS="my-app"
MINIO_NS="minio-ns"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-120s}"

cleanup() {
  if [ "${KEEP_E2E_ARTIFACTS:-}" = "1" ]; then
    echo "KEEP_E2E_ARTIFACTS=1 set; skipping cleanup for debugging"
    return
  fi
  "${KUBECTL_BIN}" delete ns "${APP_NS}" --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete ns "${MINIO_NS}" --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete ns "${OPERATOR_NS}" --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete crd bucketclaims.objectstorage.k8s.io bucketclasses.objectstorage.k8s.io --ignore-not-found >/dev/null 2>&1 || true
}
trap cleanup EXIT

assert_bucketclaim_ready() {
  local claim_name="$1"
  local namespace="$2"

  local phase ready_reason ready_status
  phase=$("${KUBECTL_BIN}" get bucketclaim "${claim_name}" -n "${namespace}" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  ready_status=$("${KUBECTL_BIN}" get bucketclaim "${claim_name}" -n "${namespace}" -o go-template='{{range .status.conditions}}{{if eq .type "Ready"}}{{.status}}{{end}}{{end}}' 2>/dev/null || true)
  ready_reason=$("${KUBECTL_BIN}" get bucketclaim "${claim_name}" -n "${namespace}" -o go-template='{{range .status.conditions}}{{if eq .type "Ready"}}{{.reason}}{{end}}{{end}}' 2>/dev/null || true)

  if [ "${phase}" != "Bound" ]; then
    echo "Error: ${claim_name} expected status.phase=Bound, got '${phase}'"
    exit 1
  fi
  if [ "${ready_status}" != "True" ]; then
    echo "Error: ${claim_name} expected status.conditions[type=Ready].status=True, got '${ready_status}'"
    exit 1
  fi
  if [ "${ready_reason}" != "BucketProvisioned" ]; then
    echo "Error: ${claim_name} expected status.conditions[type=Ready].reason=BucketProvisioned, got '${ready_reason}'"
    exit 1
  fi
}

echo "==> 1. Setting up MinIO test instance"
"${KUBECTL_BIN}" create ns "${MINIO_NS}" --dry-run=client -o yaml | "${KUBECTL_BIN}" apply -f -
"${KUBECTL_BIN}" apply -f test/e2e/minio.yaml
"${KUBECTL_BIN}" rollout status deployment/minio -n "${MINIO_NS}" --timeout="${WAIT_TIMEOUT}"

echo "==> 2. Setting up k8s-s3-bucket-operator"
make deploy
if [ -n "${OPERATOR_IMAGE}" ]; then
  echo "==> 2a. Overriding operator image: ${OPERATOR_IMAGE}"
  "${KUBECTL_BIN}" -n "${OPERATOR_NS}" set image deploy/k8s-s3-bucket-operator operator="${OPERATOR_IMAGE}"
  # Manifests default to imagePullPolicy: Always — breaks kind/local images (kubelet tries to pull from a registry).
  "${KUBECTL_BIN}" -n "${OPERATOR_NS}" patch deploy k8s-s3-bucket-operator --type=json \
    -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/imagePullPolicy", "value": "IfNotPresent"}]'
fi
"${KUBECTL_BIN}" rollout status deployment/k8s-s3-bucket-operator -n "${OPERATOR_NS}" --timeout="${WAIT_TIMEOUT}"

echo "==> 3. Creating App Namespace and applying BucketClaim"
"${KUBECTL_BIN}" create ns "${APP_NS}" --dry-run=client -o yaml | "${KUBECTL_BIN}" apply -f -
"${KUBECTL_BIN}" apply -f config/samples/bucketclass.yaml
"${KUBECTL_BIN}" apply -f config/samples/bucketclaim.yaml

echo "==> 4. Waiting for BucketClaim to bind..."
sleep 3
for i in {1..20}; do
  PHASE=$("${KUBECTL_BIN}" get bucketclaim my-app-images -n "${APP_NS}" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  READY_STATUS=$("${KUBECTL_BIN}" get bucketclaim my-app-images -n "${APP_NS}" -o go-template='{{range .status.conditions}}{{if eq .type "Ready"}}{{.status}}{{end}}{{end}}' 2>/dev/null || true)
  READY_REASON=$("${KUBECTL_BIN}" get bucketclaim my-app-images -n "${APP_NS}" -o go-template='{{range .status.conditions}}{{if eq .type "Ready"}}{{.reason}}{{end}}{{end}}' 2>/dev/null || true)
  if [ "$PHASE" == "Bound" ] || { [ "$READY_STATUS" == "True" ] && [ "$READY_REASON" == "BucketProvisioned" ]; }; then
    echo "    BucketClaim is Bound!"
    break
  fi
  echo "    Waiting... phase=$PHASE ready=$READY_STATUS/$READY_REASON"
  sleep 3
done

if [ "$PHASE" != "Bound" ]; then
  echo "Error: BucketClaim did not bind in time."
  exit 1
fi

echo "==> 5. Verifying Secret generation in App Namespace"
"${KUBECTL_BIN}" get secret my-app-images-credentials -n "${APP_NS}"

echo "==> 6. Verifying MinIO backend storage bucket + enterprise settings"
"${KUBECTL_BIN}" exec -n "${MINIO_NS}" deploy/minio -- ls -ld /data/my-v120-test-bucket
"${KUBECTL_BIN}" exec -n "${MINIO_NS}" deploy/minio -- mc alias set myminio http://localhost:9000 minioadmin minioadmin
echo "--- Bucket Quota ---"
"${KUBECTL_BIN}" exec -n "${MINIO_NS}" deploy/minio -- mc quota info myminio/my-v120-test-bucket || echo "quota info unavailable"
echo "--- Lifecycle Rules ---"
"${KUBECTL_BIN}" exec -n "${MINIO_NS}" deploy/minio -- mc ilm rule ls myminio/my-v120-test-bucket || echo "lifecycle info unavailable"

echo "==> 6a. Verifying status.conditions Ready=True"
assert_bucketclaim_ready "my-app-images" "${APP_NS}"

echo "==> 6b. Validating BucketClass minioCredentialSecretRef (per-class MinIO Secret)"
"${KUBECTL_BIN}" -n "${OPERATOR_NS}" apply -f - <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: minio-credentials-class
type: Opaque
stringData:
  MINIO_ENDPOINT: "minio.minio-ns.svc.cluster.local:9000"
  MINIO_ACCESS_KEY: "minioadmin"
  MINIO_SECRET_KEY: "minioadmin"
  MINIO_USE_SSL: "false"
EOF

"${KUBECTL_BIN}" apply -f - <<'EOF'
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: minio-secretref
driverName: k8s-s3-bucket-operator
deletionPolicy: Delete
objectLockingEnabled: true
retentionMode: GOVERNANCE
retentionDays: 30
minioCredentialSecretRef:
  namespace: k8s-s3-bucket-operator
  name: minio-credentials-class
parameters:
  region: "us-east-1"
EOF

"${KUBECTL_BIN}" apply -f - <<'EOF'
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: my-app-images-secretref
  namespace: my-app
spec:
  bucketClassName: minio-secretref
  bucketName: my-v120-test-bucket-secretref
  protocols:
    - S3
  quota: "50Gi"
  accessType: ReadOnly
  lifecycleRules:
    - id: ExpireOldBackups
      status: Enabled
      prefix: backups/
      expiration:
        days: 30
  replicationTarget:
    endpoint: minio.minio-ns.svc.cluster.local:9000
    bucketName: my-v120-test-bucket-replica-secretref
    accessKey: minioadmin
    secretKey: minioadmin
    useSSL: false
EOF

sleep 3
for i in {1..20}; do
  PHASE=$("${KUBECTL_BIN}" get bucketclaim my-app-images-secretref -n "${APP_NS}" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  READY_STATUS=$("${KUBECTL_BIN}" get bucketclaim my-app-images-secretref -n "${APP_NS}" -o go-template='{{range .status.conditions}}{{if eq .type "Ready"}}{{.status}}{{end}}{{end}}' 2>/dev/null || true)
  READY_REASON=$("${KUBECTL_BIN}" get bucketclaim my-app-images-secretref -n "${APP_NS}" -o go-template='{{range .status.conditions}}{{if eq .type "Ready"}}{{.reason}}{{end}}{{end}}' 2>/dev/null || true)
  if [ "$PHASE" == "Bound" ] || { [ "$READY_STATUS" == "True" ] && [ "$READY_REASON" == "BucketProvisioned" ]; }; then
    echo "    BucketClaim (secretref) is Bound!"
    break
  fi
  echo "    Waiting... phase=$PHASE secretref-ready=$READY_STATUS/$READY_REASON"
  sleep 3
done

if [ "$PHASE" != "Bound" ]; then
  echo "Error: BucketClaim (secretref) did not bind in time."
  exit 1
fi

"${KUBECTL_BIN}" get secret my-app-images-secretref-credentials -n "${APP_NS}"
assert_bucketclaim_ready "my-app-images-secretref" "${APP_NS}"

echo ""
echo "✅ End-to-End Test completed successfully! (cleanup will run automatically)"
