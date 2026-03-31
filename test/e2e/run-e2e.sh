#!/bin/bash
set -euo pipefail

KUBECTL_BIN="${KUBECTL:-kubectl}"
OPERATOR_NS="k8s-s3-bucket-operator"
APP_NS="my-app"
MINIO_NS="minio-ns"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-120s}"
# COSI adds an extra container + may need to pull controller/sidecar images; give rollouts more time than Wave 1.
COSI_WAIT_TIMEOUT="${COSI_WAIT_TIMEOUT:-300s}"

# COSI status JSONPath helpers (keep in one place for easier API evolution).
COSI_JSONPATH_BUCKETCLAIM_READY=".status.bucketReady"
COSI_JSONPATH_BUCKET_READY=".status.bucketReady"
COSI_JSONPATH_BUCKETACCESS_GRANTED=".status.accessGranted"

cleanup() {
  if [ "${KEEP_E2E_ARTIFACTS:-}" = "1" ]; then
    echo "KEEP_E2E_ARTIFACTS=1 set; skipping cleanup for debugging"
    return
  fi
  "${KUBECTL_BIN}" delete ns "${APP_NS}" --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete ns "${MINIO_NS}" --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete ns "${OPERATOR_NS}" --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete crd bucketclaims.objectstorage.k8s.io bucketclasses.objectstorage.k8s.io --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete crd buckets.objectstorage.k8s.io bucketaccesses.objectstorage.k8s.io bucketaccessclasses.objectstorage.k8s.io --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete clusterrole objectstorage-controller-role --ignore-not-found >/dev/null 2>&1 || true
  "${KUBECTL_BIN}" delete clusterrolebinding objectstorage-controller --ignore-not-found >/dev/null 2>&1 || true
}
trap cleanup EXIT

patch_operator_for_cosi() {
  echo "==> Patching operator Deployment for COSI (driver env + sidecar + RBAC)"
  "${KUBECTL_BIN}" apply -f deploy/cosi/controller.yaml

  # Extend operator ClusterRole with COSI sidecar permissions.
  "${KUBECTL_BIN}" patch clusterrole k8s-s3-bucket-operator-role --type=json -p='[
    {"op":"add","path":"/rules/-","value":{
      "apiGroups":["objectstorage.k8s.io"],
      "resources":["buckets","buckets/status","bucketaccesses","bucketaccesses/status","bucketaccessclasses"],
      "verbs":["get","list","watch","create","update","patch","delete"]
    }}
  ]'

  "${KUBECTL_BIN}" get deploy k8s-s3-bucket-operator -n "${OPERATOR_NS}" -o json | python3 -c 'import json,sys,os; d=json.load(sys.stdin); tpl=d.setdefault("spec",{}).setdefault("template",{}); spec=tpl.setdefault("spec",{});
vols=spec.setdefault("volumes",[]);
cosi_vol={"name":"cosi-socket","emptyDir":{}};
vols[:] = [v for v in vols if v.get("name")!="cosi-socket"]; vols.append(cosi_vol);
containers=spec.setdefault("containers",[]);
op=next(c for c in containers if c.get("name")=="operator");
vm=op.setdefault("volumeMounts",[]);
vm[:] = [m for m in vm if m.get("name")!="cosi-socket"]; vm.append({"name":"cosi-socket","mountPath":"/var/lib/cosi"});
env=op.setdefault("env",[]);

def upsert_env(name,value):
  for e in env:
    if e.get("name")==name:
      e["value"]=value
      return
  env.append({"name":name,"value":value})

upsert_env("COSI_ENABLED","true");
upsert_env("COSI_DRIVER_NAME","k8s-s3-bucket-operator");
upsert_env("COSI_SOCKET_PATH","/var/lib/cosi/cosi.sock");
sidecar_image=os.environ.get("COSI_SIDECAR_IMAGE","gcr.io/k8s-staging-sig-storage/objectstorage-sidecar:v20240513-v0.1.0-35-gefb3255");
sidecar_def={"name":"objectstorage-sidecar","image":sidecar_image,"imagePullPolicy":"IfNotPresent","args":["--driver-addr=unix:///var/lib/cosi/cosi.sock","--v=3"],"securityContext":{"allowPrivilegeEscalation":False,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":True},"volumeMounts":[{"name":"cosi-socket","mountPath":"/var/lib/cosi"}],"resources":{"requests":{"cpu":"25m","memory":"64Mi"},"limits":{"cpu":"200m","memory":"256Mi"}}};
sidecar=next((c for c in containers if c.get("name")=="objectstorage-sidecar"), None);
if sidecar is None:
  containers.append(sidecar_def);
else:
  sidecar.update(sidecar_def);
json.dump(d, sys.stdout)
' | "${KUBECTL_BIN}" apply -f -
}

wait_jsonpath_true() {
  local kind="$1"
  local namespace="$2"
  local name="$3"
  local jsonpath="$4"
  local timeout_seconds="${5:-240}"
  local deadline=$((SECONDS+timeout_seconds))

  while [ $SECONDS -lt $deadline ]; do
    local v
    if [ -n "${namespace}" ]; then
      v="$("${KUBECTL_BIN}" get "${kind}" "${name}" -n "${namespace}" -o "jsonpath={${jsonpath}}" 2>/dev/null || true)"
    else
      v="$("${KUBECTL_BIN}" get "${kind}" "${name}" -o "jsonpath={${jsonpath}}" 2>/dev/null || true)"
    fi
    if [ "${v}" = "true" ]; then
      return 0
    fi
    sleep 3
  done

  echo "Error: timed out waiting for ${kind}/${name} (${jsonpath})"
  return 1
}

assert_minio_user_exists() {
  local access_key="$1"
  "${KUBECTL_BIN}" exec -n "${MINIO_NS}" deploy/minio -- mc admin user info myminio "${access_key}" >/dev/null 2>&1
}

assert_bucketclaim_ready() {
  local claim_name="$1"
  local namespace="$2"

  local phase ready_reason ready_status
  IFS=$'\t' read -r phase ready_status ready_reason <<< "$("${KUBECTL_BIN}" get bucketclaim "${claim_name}" -n "${namespace}" -o json 2>/dev/null | python3 -c 'import json,sys; data=sys.stdin.read(); o=json.loads(data) if data else {}; st=o.get("status",{}) or {}; phase=st.get("phase","") or ""; conds=st.get("conditions",[]) or []; cond=next((c for c in conds if (c.get("type")=="Ready" or c.get("Type")=="Ready")), {}); rs=cond.get("status","") or ""; rr=cond.get("reason","") or ""; sys.stdout.write(phase+"\t"+rs+"\t"+rr)' 2>/dev/null || true)"

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

wait_bucket_gone() {
  local bucket_name="$1"
  local bucket_dir="/data/${bucket_name}"
  local timeout_seconds="${2:-180}"
  local deadline=$((SECONDS+timeout_seconds))

  while [ $SECONDS -lt $deadline ]; do
    if ! "${KUBECTL_BIN}" exec -n "${MINIO_NS}" deploy/minio -- ls -ld "${bucket_dir}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 3
  done

  echo "Error: MinIO bucket directory ${bucket_dir} still exists"
  return 1
}

wait_namespace_active() {
  local namespace="$1"
  local timeout_seconds="${2:-180}"
  local deadline=$((SECONDS+timeout_seconds))

  # If a previous run is still terminating the namespace, wait until it is gone.
  while [ $SECONDS -lt $deadline ]; do
    ts="$("${KUBECTL_BIN}" get ns "${namespace}" -o jsonpath='{.metadata.deletionTimestamp}' 2>/dev/null || true)"
    if [ -z "${ts}" ]; then
      # Either namespace is active (no deletionTimestamp) or it doesn't exist.
      return 0
    fi
    sleep 3
  done

  echo "Error: namespace ${namespace} is still terminating"
  return 1
}

wait_resource_gone() {
  local kind="$1"
  local namespace="$2"
  local name="$3"
  local timeout_seconds="${4:-120}"
  local deadline=$((SECONDS+timeout_seconds))

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
    sleep 2
  done

  echo "Error: ${kind} ${name} still exists (namespace='${namespace}')"
  return 1
}

delete_bucketclaim_and_assert_cleanup() {
  local claim_name="$1"
  local namespace="$2"
  local secret_name="$3"
  local bucket_name="$4"

  echo "==> Deleting BucketClaim ${claim_name} (expect secret + bucket cleanup)"
  # Tests can be rerun after partial cleanup; treat NotFound as already-deleted and continue
  # so we still validate that Secret + backend bucket are gone.
  "${KUBECTL_BIN}" delete bucketclaim "${claim_name}" -n "${namespace}" --wait=false >/dev/null 2>&1 || true

  # Finalizer removal is asynchronous; validate via GC for secret and bucket delete in MinIO.
  wait_secret_gone "${secret_name}" "${namespace}" 180
  wait_bucket_gone "${bucket_name}" 240
}

echo "==> 1. Setting up MinIO test instance"
wait_namespace_active "${MINIO_NS}" 240
"${KUBECTL_BIN}" create ns "${MINIO_NS}" --dry-run=client -o yaml | "${KUBECTL_BIN}" apply -f -
"${KUBECTL_BIN}" apply -f test/e2e/minio.yaml
"${KUBECTL_BIN}" rollout status deployment/minio -n "${MINIO_NS}" --timeout="${WAIT_TIMEOUT}"

echo "==> 2. Setting up k8s-s3-bucket-operator"
wait_namespace_active "${OPERATOR_NS}" 240
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
wait_namespace_active "${APP_NS}" 240
"${KUBECTL_BIN}" create ns "${APP_NS}" --dry-run=client -o yaml | "${KUBECTL_BIN}" apply -f -
"${KUBECTL_BIN}" apply -f config/samples/bucketclass.yaml
"${KUBECTL_BIN}" apply -f config/samples/bucketclaim.yaml

echo "==> 4. Waiting for BucketClaim to bind..."
sleep 3
for i in {1..60}; do
  IFS=$'\t' read -r PHASE READY_STATUS READY_REASON <<< "$("${KUBECTL_BIN}" get bucketclaim my-app-images -n "${APP_NS}" -o json 2>/dev/null | python3 -c 'import json,sys; data=sys.stdin.read(); o=json.loads(data) if data else {}; st=o.get("status",{}) or {}; phase=st.get("phase","") or ""; conds=st.get("conditions",[]) or []; cond=next((c for c in conds if (c.get("type")=="Ready" or c.get("Type")=="Ready")), {}); rs=cond.get("status","") or ""; rr=cond.get("reason","") or ""; sys.stdout.write(phase+"\t"+rs+"\t"+rr)' 2>/dev/null || true)"
  if [ "$PHASE" == "Bound" ] && [ "$READY_STATUS" == "True" ] && [ "$READY_REASON" == "BucketProvisioned" ]; then
    echo "    BucketClaim is Bound!"
    break
  fi
  echo "    Waiting... phase=$PHASE ready=$READY_STATUS/$READY_REASON"
  sleep 3
done

if [ "$PHASE" != "Bound" ] || [ "$READY_STATUS" != "True" ] || [ "$READY_REASON" != "BucketProvisioned" ]; then
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
for i in {1..60}; do
  IFS=$'\t' read -r PHASE READY_STATUS READY_REASON <<< "$("${KUBECTL_BIN}" get bucketclaim my-app-images-secretref -n "${APP_NS}" -o json 2>/dev/null | python3 -c 'import json,sys; data=sys.stdin.read(); o=json.loads(data) if data else {}; st=o.get("status",{}) or {}; phase=st.get("phase","") or ""; conds=st.get("conditions",[]) or []; cond=next((c for c in conds if (c.get("type")=="Ready" or c.get("Type")=="Ready")), {}); rs=cond.get("status","") or ""; rr=cond.get("reason","") or ""; sys.stdout.write(phase+"\t"+rs+"\t"+rr)' 2>/dev/null || true)"
  if [ "$PHASE" == "Bound" ] && [ "$READY_STATUS" == "True" ] && [ "$READY_REASON" == "BucketProvisioned" ]; then
    echo "    BucketClaim (secretref) is Bound!"
    break
  fi
  echo "    Waiting... phase=$PHASE secretref-ready=$READY_STATUS/$READY_REASON"
  sleep 3
done

if [ "$PHASE" != "Bound" ] || [ "$READY_STATUS" != "True" ] || [ "$READY_REASON" != "BucketProvisioned" ]; then
  echo "Error: BucketClaim (secretref) did not bind in time."
  exit 1
fi

"${KUBECTL_BIN}" get secret my-app-images-secretref-credentials -n "${APP_NS}"
assert_bucketclaim_ready "my-app-images-secretref" "${APP_NS}"

echo "==> 7. Verifying finalizer cleanup on BucketClaim deletion"
delete_bucketclaim_and_assert_cleanup "my-app-images" "${APP_NS}" "my-app-images-credentials" "my-v120-test-bucket"
delete_bucketclaim_and_assert_cleanup "my-app-images-secretref" "${APP_NS}" "my-app-images-secretref-credentials" "my-v120-test-bucket-secretref"

echo ""
echo "==> 8. COSI (optional): CRDs + controller + driver wiring + access lifecycle"
"${KUBECTL_BIN}" apply -k deploy/cosi
patch_operator_for_cosi
"${KUBECTL_BIN}" rollout status deployment/k8s-s3-bucket-operator -n "${OPERATOR_NS}" --timeout="${COSI_WAIT_TIMEOUT}"
"${KUBECTL_BIN}" rollout status deployment/objectstorage-controller -n "${OPERATOR_NS}" --timeout="${COSI_WAIT_TIMEOUT}"

RUN_ID="$(date +%s)"
COSI_BUCKET_NAME="cosi-e2e-bucket-${RUN_ID}"
COSI_BUCKET_DIR="/data/${COSI_BUCKET_NAME}"
COSI_BUCKETCLASS="cosi-minio-standard-${RUN_ID}"
COSI_BUCKETACCESSCLASS="cosi-minio-keys-${RUN_ID}"
COSI_BUCKETCLAIM="cosi-app-bucket-${RUN_ID}"
COSI_BUCKETACCESS="cosi-app-access-${RUN_ID}"
COSI_CREDS_SECRET="cosi-app-creds-${RUN_ID}"

"${KUBECTL_BIN}" apply -f - <<EOF
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: ${COSI_BUCKETCLASS}
driverName: k8s-s3-bucket-operator
deletionPolicy: Delete
parameters:
  region: "us-east-1"
  bucketName: "${COSI_BUCKET_NAME}"
---
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketAccessClass
metadata:
  name: ${COSI_BUCKETACCESSCLASS}
driverName: k8s-s3-bucket-operator
authenticationType: Key
---
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: ${COSI_BUCKETCLAIM}
  namespace: ${APP_NS}
spec:
  bucketClassName: ${COSI_BUCKETCLASS}
  protocols:
    - S3
---
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketAccess
metadata:
  name: ${COSI_BUCKETACCESS}
  namespace: ${APP_NS}
spec:
  bucketAccessClassName: ${COSI_BUCKETACCESSCLASS}
  bucketClaimName: ${COSI_BUCKETCLAIM}
  credentialsSecretName: ${COSI_CREDS_SECRET}
  protocol: S3
EOF

wait_jsonpath_true "bucketclaim" "${APP_NS}" "${COSI_BUCKETCLAIM}" "${COSI_JSONPATH_BUCKETCLAIM_READY}" 300

BUCKET_OBJ="$("${KUBECTL_BIN}" get bucketclaim "${COSI_BUCKETCLAIM}" -n "${APP_NS}" -o jsonpath="{.status.bucketName}")"
if [ -z "${BUCKET_OBJ}" ]; then
  echo "Error: COSI BucketClaim did not populate status.bucketName"
  exit 1
fi

wait_jsonpath_true "bucketaccess" "${APP_NS}" "${COSI_BUCKETACCESS}" "${COSI_JSONPATH_BUCKETACCESS_GRANTED}" 300
"${KUBECTL_BIN}" get secret "${COSI_CREDS_SECRET}" -n "${APP_NS}"

ACCESS_KEY="$("${KUBECTL_BIN}" get secret "${COSI_CREDS_SECRET}" -n "${APP_NS}" -o jsonpath='{.data.BucketInfo}' | python3 -c 'import base64,json,sys; raw=sys.stdin.read().strip();
if not raw: raise SystemExit("missing .data.BucketInfo")
obj=json.loads(base64.b64decode(raw).decode("utf-8"))
print(obj["spec"]["s3"]["accessKeyID"])')"

"${KUBECTL_BIN}" exec -n "${MINIO_NS}" deploy/minio -- mc alias set myminio http://localhost:9000 minioadmin minioadmin >/dev/null
if ! assert_minio_user_exists "${ACCESS_KEY}"; then
  echo "Error: expected MinIO user ${ACCESS_KEY} to exist after BucketAccess grant"
  exit 1
fi

if ! "${KUBECTL_BIN}" exec -n "${MINIO_NS}" deploy/minio -- ls -ld "${COSI_BUCKET_DIR}" >/dev/null 2>&1; then
  echo "Error: expected MinIO bucket dir ${COSI_BUCKET_DIR} to exist"
  exit 1
fi

echo "==> 8a. Deleting BucketAccess should revoke credentials but keep bucket"
"${KUBECTL_BIN}" delete bucketaccess "${COSI_BUCKETACCESS}" -n "${APP_NS}" --wait=true
wait_secret_gone "${COSI_CREDS_SECRET}" "${APP_NS}" 240
if assert_minio_user_exists "${ACCESS_KEY}"; then
  echo "Error: expected MinIO user ${ACCESS_KEY} to be removed after BucketAccess deletion"
  exit 1
fi
if ! "${KUBECTL_BIN}" exec -n "${MINIO_NS}" deploy/minio -- ls -ld "${COSI_BUCKET_DIR}" >/dev/null 2>&1; then
  echo "Error: expected bucket to still exist after BucketAccess deletion"
  exit 1
fi

echo "==> 8b. Deleting BucketClaim should delete bucket when BucketClass deletionPolicy=Delete"
"${KUBECTL_BIN}" delete bucketclaim "${COSI_BUCKETCLAIM}" -n "${APP_NS}" --wait=true
wait_bucket_gone "${COSI_BUCKET_NAME}" 300

echo ""
echo "✅ End-to-End Test completed successfully! (cleanup will run automatically)"
