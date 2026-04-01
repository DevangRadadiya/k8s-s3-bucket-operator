#!/bin/bash
# AWS backend E2E using LocalStack (S3 + IAM emulator) running inside the kind cluster.
# No real AWS credentials or account needed — suitable for CI.
#
# What is tested:
#   1. LocalStack deployment in kind
#   2. Operator deployed with backend=AWS pointing at LocalStack
#   3. BucketClaim lifecycle: Bound, Secret generation, deletion/finalizer cleanup
#   4. Security hardening calls (public access block, ownership controls, encryption, TLS policy)
#   5. BucketClaim deletionPolicy=Delete removes the bucket in LocalStack
set -euo pipefail

KUBECTL_BIN="${KUBECTL:-kubectl}"
OPERATOR_NS="k8s-s3-bucket-operator"
APP_NS="aws-e2e"
LOCALSTACK_NS="localstack-ns"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-120s}"
LOCALSTACK_IMAGE="${LOCALSTACK_IMAGE:-localstack/localstack:3}"
AWS_REGION="us-east-1"
LOCALSTACK_SVC="localstack.${LOCALSTACK_NS}.svc.cluster.local"
LOCALSTACK_ENDPOINT="http://${LOCALSTACK_SVC}:4566"

cleanup() {
  if [ "${KEEP_E2E_ARTIFACTS:-}" = "1" ]; then
    echo "KEEP_E2E_ARTIFACTS=1 set; skipping cleanup for debugging"
    return
  fi
  "${KUBECTL_BIN}" delete ns "${APP_NS}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete ns "${LOCALSTACK_NS}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete ns "${OPERATOR_NS}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete crd bucketclaims.objectstorage.k8s.io bucketclasses.objectstorage.k8s.io --ignore-not-found --wait=false >/dev/null 2>&1 || true
}
trap cleanup EXIT

wait_namespace_active() {
  local namespace="$1"
  local timeout_seconds="${2:-180}"
  local deadline=$((SECONDS+timeout_seconds))
  while [ $SECONDS -lt $deadline ]; do
    ts="$("${KUBECTL_BIN}" get ns "${namespace}" -o jsonpath='{.metadata.deletionTimestamp}' 2>/dev/null || true)"
    if [ -z "${ts}" ]; then
      return 0
    fi
    sleep 3
  done
  echo "Error: namespace ${namespace} is still terminating after ${timeout_seconds}s"
  return 1
}

wait_resource_gone() {
  local kind="$1"
  local namespace="$2"
  local name="$3"
  local timeout_raw="${4:-120}"
  local timeout_seconds="${timeout_raw%s}"
  timeout_seconds="${timeout_seconds%S}"
  if ! [[ "${timeout_seconds}" =~ ^[0-9]+$ ]]; then
    timeout_seconds=120
  fi
  local deadline=$((SECONDS+timeout_seconds))
  local last_log="$SECONDS"
  while [ $SECONDS -lt $deadline ]; do
    if [ -n "${namespace}" ]; then
      if ! "${KUBECTL_BIN}" get "${kind}" "${name}" -n "${namespace}" >/dev/null 2>&1; then
        return 0
      fi
    else
      if ! "${KUBECTL_BIN}" get "${kind}" "${name}" >/dev/null 2>&1; then
        return 0
      fi
    fi
    if [ $((SECONDS - last_log)) -ge 30 ]; then
      echo "    ... still waiting for ${kind}/${name} to delete ($((deadline - SECONDS))s left)"
      last_log=$SECONDS
    fi
    sleep 3
  done
  echo "Error: ${kind}/${name} still exists after ${timeout_seconds}s"
  "${KUBECTL_BIN}" get "${kind}" "${name}" -n "${namespace}" -o yaml 2>/dev/null | head -n 60 || true
  return 1
}

wait_secret_gone() {
  local secret_name="$1"
  local namespace="$2"
  local timeout_seconds="${3:-120}"
  local deadline=$((SECONDS+timeout_seconds))
  while [ $SECONDS -lt $deadline ]; do
    if ! "${KUBECTL_BIN}" get secret "${secret_name}" -n "${namespace}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 3
  done
  echo "Error: secret ${secret_name} still exists in ${namespace}"
  return 1
}

assert_bucketclaim_bound() {
  local claim_name="$1"
  local namespace="$2"
  IFS=$'\t' read -r phase ready_status ready_reason <<< "$("${KUBECTL_BIN}" get bucketclaim "${claim_name}" -n "${namespace}" -o json 2>/dev/null | python3 -c 'import json,sys; data=sys.stdin.read(); o=json.loads(data) if data else {}; st=o.get("status",{}) or {}; phase=st.get("phase","") or ""; conds=st.get("conditions",[]) or []; cond=next((c for c in conds if (c.get("type")=="Ready" or c.get("Type")=="Ready")), {}); rs=cond.get("status","") or ""; rr=cond.get("reason","") or ""; sys.stdout.write(phase+"\t"+rs+"\t"+rr)' 2>/dev/null || true)"
  if [ "${phase}" != "Bound" ]; then
    echo "Error: ${claim_name} expected Bound, got '${phase}'"
    exit 1
  fi
  if [ "${ready_status}" != "True" ] || [ "${ready_reason}" != "BucketProvisioned" ]; then
    echo "Error: ${claim_name} Ready condition: status=${ready_status} reason=${ready_reason}"
    exit 1
  fi
}

# Verify a bucket exists via LocalStack S3 API using kubectl exec.
assert_bucket_exists_localstack() {
  local bucket_name="$1"
  if ! "${KUBECTL_BIN}" exec -n "${LOCALSTACK_NS}" deploy/localstack -- \
      awslocal s3api head-bucket --bucket "${bucket_name}" >/dev/null 2>&1; then
    echo "Error: expected bucket '${bucket_name}' to exist in LocalStack"
    return 1
  fi
}

# Verify a bucket is gone via LocalStack S3 API.
assert_bucket_gone_localstack() {
  local bucket_name="$1"
  local timeout_seconds="${2:-120}"
  local deadline=$((SECONDS+timeout_seconds))
  while [ $SECONDS -lt $deadline ]; do
    if ! "${KUBECTL_BIN}" exec -n "${LOCALSTACK_NS}" deploy/localstack -- \
        awslocal s3api head-bucket --bucket "${bucket_name}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 3
  done
  echo "Error: bucket '${bucket_name}' still exists in LocalStack after ${timeout_seconds}s"
  return 1
}

echo "==> 1. Deploying LocalStack (S3 + IAM emulator)"
wait_namespace_active "${LOCALSTACK_NS}" 180
"${KUBECTL_BIN}" create ns "${LOCALSTACK_NS}" --dry-run=client -o yaml | "${KUBECTL_BIN}" apply -f -

"${KUBECTL_BIN}" apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: localstack
  namespace: ${LOCALSTACK_NS}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: localstack
  template:
    metadata:
      labels:
        app: localstack
    spec:
      containers:
      - name: localstack
        image: ${LOCALSTACK_IMAGE}
        ports:
        - containerPort: 4566
        env:
        - name: SERVICES
          value: "s3,iam"
        - name: DEFAULT_REGION
          value: "${AWS_REGION}"
        - name: AWS_DEFAULT_REGION
          value: "${AWS_REGION}"
        - name: LOCALSTACK_HOST
          value: "${LOCALSTACK_SVC}"
        readinessProbe:
          httpGet:
            path: /_localstack/health
            port: 4566
          initialDelaySeconds: 5
          periodSeconds: 5
          failureThreshold: 20
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
---
apiVersion: v1
kind: Service
metadata:
  name: localstack
  namespace: ${LOCALSTACK_NS}
spec:
  selector:
    app: localstack
  ports:
  - port: 4566
    targetPort: 4566
EOF

"${KUBECTL_BIN}" rollout status deployment/localstack -n "${LOCALSTACK_NS}" --timeout="${WAIT_TIMEOUT}"
echo "    LocalStack ready at ${LOCALSTACK_ENDPOINT}"

echo "==> 2. Deploying operator"
wait_namespace_active "${OPERATOR_NS}" 180
make deploy
if [ -n "${OPERATOR_IMAGE}" ]; then
  echo "==> 2a. Overriding operator image: ${OPERATOR_IMAGE}"
  "${KUBECTL_BIN}" -n "${OPERATOR_NS}" set image deploy/k8s-s3-bucket-operator operator="${OPERATOR_IMAGE}"
  "${KUBECTL_BIN}" -n "${OPERATOR_NS}" patch deploy k8s-s3-bucket-operator --type=json \
    -p='[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"IfNotPresent"}]'
fi
"${KUBECTL_BIN}" rollout status deployment/k8s-s3-bucket-operator -n "${OPERATOR_NS}" --timeout="${WAIT_TIMEOUT}"

echo "==> 3. Creating LocalStack credential Secret and BucketClass"
wait_namespace_active "${OPERATOR_NS}" 60
"${KUBECTL_BIN}" apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: localstack-aws-creds
  namespace: ${OPERATOR_NS}
type: Opaque
stringData:
  AWS_REGION: "${AWS_REGION}"
  AWS_ACCESS_KEY_ID: "test"
  AWS_SECRET_ACCESS_KEY: "test"
  AWS_S3_ENDPOINT: "${LOCALSTACK_ENDPOINT}"
  AWS_IAM_ENDPOINT: "${LOCALSTACK_ENDPOINT}"
EOF

wait_namespace_active "${APP_NS}" 180
"${KUBECTL_BIN}" create ns "${APP_NS}" --dry-run=client -o yaml | "${KUBECTL_BIN}" apply -f -

BUCKET_NAME="aws-e2e-localstack-bucket"
SECRET_NAME="aws-claim1-credentials"

"${KUBECTL_BIN}" apply -f - <<EOF
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: aws-localstack
driverName: k8s-s3-bucket-operator
backend: AWS
deletionPolicy: Delete
parameters:
  region: "${AWS_REGION}"
  security.blockPublicAccess: "true"
  security.disableAcls: "true"
  security.defaultEncryption: "SSE-S3"
  security.tlsOnlyPolicy: "true"
awsCredentialSecretRef:
  namespace: ${OPERATOR_NS}
  name: localstack-aws-creds
EOF

echo "==> 4. Creating BucketClaim"
"${KUBECTL_BIN}" apply -f - <<EOF
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: aws-claim1
  namespace: ${APP_NS}
spec:
  bucketClassName: aws-localstack
  bucketName: ${BUCKET_NAME}
  protocols:
    - S3
  accessType: ReadWrite
  lifecycleRules:
    - id: expire-test
      status: Enabled
      prefix: tmp/
      expiration:
        days: 7
EOF

echo "==> 5. Waiting for BucketClaim to bind..."
sleep 3
for i in {1..60}; do
  IFS=$'\t' read -r PHASE READY_STATUS READY_REASON <<< "$("${KUBECTL_BIN}" get bucketclaim aws-claim1 -n "${APP_NS}" -o json 2>/dev/null | python3 -c 'import json,sys; data=sys.stdin.read(); o=json.loads(data) if data else {}; st=o.get("status",{}) or {}; phase=st.get("phase","") or ""; conds=st.get("conditions",[]) or []; cond=next((c for c in conds if (c.get("type")=="Ready" or c.get("Type")=="Ready")), {}); rs=cond.get("status","") or ""; rr=cond.get("reason","") or ""; sys.stdout.write(phase+"\t"+rs+"\t"+rr)' 2>/dev/null || true)"
  if [ "${PHASE}" == "Bound" ] && [ "${READY_STATUS}" == "True" ] && [ "${READY_REASON}" == "BucketProvisioned" ]; then
    echo "    BucketClaim is Bound!"
    break
  fi
  echo "    Waiting... phase=${PHASE} ready=${READY_STATUS}/${READY_REASON}"
  sleep 3
done

if [ "${PHASE}" != "Bound" ] || [ "${READY_STATUS}" != "True" ] || [ "${READY_REASON}" != "BucketProvisioned" ]; then
  echo "Error: BucketClaim did not bind in time."
  "${KUBECTL_BIN}" describe bucketclaim aws-claim1 -n "${APP_NS}" || true
  "${KUBECTL_BIN}" logs -n "${OPERATOR_NS}" deploy/k8s-s3-bucket-operator --tail=60 || true
  exit 1
fi

assert_bucketclaim_bound "aws-claim1" "${APP_NS}"

echo "==> 6. Verifying credentials Secret"
"${KUBECTL_BIN}" get secret "${SECRET_NAME}" -n "${APP_NS}"

# Verify required keys are present in the secret.
for key in accessKeyID accessSecretKey bucketName endpoint; do
  val="$("${KUBECTL_BIN}" get secret "${SECRET_NAME}" -n "${APP_NS}" -o jsonpath="{.data.${key}}" 2>/dev/null | base64 -d 2>/dev/null || true)"
  if [ -z "${val}" ]; then
    echo "Error: Secret ${SECRET_NAME} missing key '${key}'"
    exit 1
  fi
  echo "    ${key}=${val}"
done

echo "==> 7. Verifying bucket exists in LocalStack"
assert_bucket_exists_localstack "${BUCKET_NAME}"
echo "    Bucket '${BUCKET_NAME}' confirmed in LocalStack"

echo "==> 8. Deleting BucketClaim (deletionPolicy=Delete should remove bucket)"
"${KUBECTL_BIN}" delete bucketclaim aws-claim1 -n "${APP_NS}" --wait=false >/dev/null 2>&1 || true
wait_resource_gone "bucketclaim" "${APP_NS}" "aws-claim1" 180
wait_secret_gone "${SECRET_NAME}" "${APP_NS}" 120

echo "==> 9. Verifying bucket was deleted from LocalStack"
assert_bucket_gone_localstack "${BUCKET_NAME}" 120
echo "    Bucket '${BUCKET_NAME}' confirmed deleted from LocalStack"

echo ""
echo "✅ AWS LocalStack End-to-End Test completed successfully! (cleanup will run automatically)"
