#!/usr/bin/env bash
set -euo pipefail

# Real AWS smoke test against a live cluster.
#
# Requirements:
# - kubectl context points to the target cluster
# - BucketClass aws-standard exists (backend=AWS + awsCredentialSecretRef)
# - aws CLI is configured (for verification only)

NS="${NS:-aws-test}"
CLAIM="${CLAIM:-claim1}"
CLASS="${CLASS:-aws-standard}"

# Match operator config: region from BucketClass.parameters.region, else Secret AWS_REGION.
resolve_aws_region() {
  local r
  r="$(kubectl get bucketclass "${CLASS}" -o jsonpath='{.spec.parameters.region}' 2>/dev/null || true)"
  if [[ -z "${r}" ]]; then
    local cref nsref nameref
    nsref="$(kubectl get bucketclass "${CLASS}" -o jsonpath='{.spec.awsCredentialSecretRef.namespace}' 2>/dev/null || true)"
    nameref="$(kubectl get bucketclass "${CLASS}" -o jsonpath='{.spec.awsCredentialSecretRef.name}' 2>/dev/null || true)"
    if [[ -n "${nsref}" && -n "${nameref}" ]]; then
      r="$(kubectl -n "${nsref}" get secret "${nameref}" -o jsonpath='{.data.AWS_REGION}' 2>/dev/null | base64 -d || true)"
    fi
  fi
  if [[ -z "${r}" ]]; then
    r="${AWS_REGION:-${AWS_DEFAULT_REGION:-}}"
  fi
  if [[ -z "${r}" ]]; then
    echo "WARN: could not read region from BucketClass ${CLASS} or its Secret; aws verify may use wrong region" >&2
    r="us-east-1"
  fi
  echo "${r}"
}

AWS_VERIFY_REGION="$(resolve_aws_region)"
export AWS_DEFAULT_REGION="${AWS_VERIFY_REGION}"
echo "==> AWS verify region (from class/secret): ${AWS_VERIFY_REGION}"

echo "==> Ensuring namespace ${NS}"
kubectl create ns "${NS}" --dry-run=client -o yaml | kubectl apply -f -

echo "==> Applying BucketClaim ${NS}/${CLAIM} (class=${CLASS})"
cat <<EOF | kubectl -n "${NS}" apply -f -
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: ${CLAIM}
spec:
  bucketClassName: ${CLASS}
  protocols:
    - S3
  accessType: ReadWrite
  lifecycleRules:
    - id: expire-test
      status: Enabled
      prefix: test/
      expiration:
        days: 7
EOF

echo "==> Waiting for credentials secret ${CLAIM}-credentials"
kubectl -n "${NS}" wait --for=condition=Ready bucketclaim/"${CLAIM}" --timeout=5m

BUCKET="$(kubectl -n "${NS}" get secret "${CLAIM}-credentials" -o jsonpath='{.data.bucketName}' | base64 -d)"
echo "==> Bucket: ${BUCKET}"

if command -v aws >/dev/null 2>&1; then
  echo "==> Verifying bucket exists in AWS (region ${AWS_VERIFY_REGION})"
  aws s3api head-bucket --bucket "${BUCKET}" --region "${AWS_VERIFY_REGION}"
else
  echo "==> aws CLI not found; skipping AWS verification"
fi

echo "==> Deleting BucketClaim ${NS}/${CLAIM}"
kubectl -n "${NS}" delete bucketclaim "${CLAIM}"

if command -v aws >/dev/null 2>&1; then
  echo "==> Verifying bucket was deleted (deletionPolicy must be Delete)"
  if aws s3api head-bucket --bucket "${BUCKET}" --region "${AWS_VERIFY_REGION}" >/dev/null 2>&1; then
    echo "Bucket still exists: ${BUCKET}"
    exit 1
  fi
  echo "Bucket deleted: ${BUCKET}"
fi

echo "==> Done"

