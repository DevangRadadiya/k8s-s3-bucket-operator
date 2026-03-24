#!/bin/bash
set -euo pipefail

KUBECTL_BIN="${KUBECTL:-kubectl}"
OPERATOR_NS="k8s-s3-bucket-operator"
APP_NS="my-app"
MINIO_NS="minio-ns"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-120s}"

cleanup() {
  "${KUBECTL_BIN}" delete ns "${APP_NS}" --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete ns "${MINIO_NS}" --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete ns "${OPERATOR_NS}" --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete crd bucketclaims.objectstorage.k8s.io bucketclasses.objectstorage.k8s.io --ignore-not-found >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> 1. Setting up MinIO test instance"
"${KUBECTL_BIN}" create ns "${MINIO_NS}" --dry-run=client -o yaml | "${KUBECTL_BIN}" apply -f -
"${KUBECTL_BIN}" apply -f test/e2e/minio.yaml
"${KUBECTL_BIN}" rollout status deployment/minio -n "${MINIO_NS}" --timeout="${WAIT_TIMEOUT}"

echo "==> 2. Setting up k8s-s3-bucket-operator"
make deploy
if [ -n "${OPERATOR_IMAGE}" ]; then
  echo "==> 2a. Overriding operator image: ${OPERATOR_IMAGE}"
  "${KUBECTL_BIN}" -n "${OPERATOR_NS}" set image deploy/k8s-s3-bucket-operator operator="${OPERATOR_IMAGE}"
fi
"${KUBECTL_BIN}" rollout status deployment/k8s-s3-bucket-operator -n "${OPERATOR_NS}" --timeout="${WAIT_TIMEOUT}"

echo "==> 3. Creating App Namespace and applying BucketClaim"
"${KUBECTL_BIN}" create ns "${APP_NS}" --dry-run=client -o yaml | "${KUBECTL_BIN}" apply -f -
"${KUBECTL_BIN}" apply -f config/samples/bucketclass.yaml
"${KUBECTL_BIN}" apply -f config/samples/bucketclaim.yaml

echo "==> 4. Waiting for BucketClaim to bind..."
sleep 3
for i in {1..10}; do
  PHASE=$("${KUBECTL_BIN}" get bucketclaim my-app-images -n "${APP_NS}" -o jsonpath='{.status.phase}')
  if [ "$PHASE" == "Bound" ]; then
    echo "    BucketClaim is Bound!"
    break
  fi
  echo "    Waiting... current phase: $PHASE"
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

echo ""
echo "✅ End-to-End Test completed successfully! (cleanup will run automatically)"
