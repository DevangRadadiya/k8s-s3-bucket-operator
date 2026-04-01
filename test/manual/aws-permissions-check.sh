#!/usr/bin/env bash
# Verify IAM permissions using simulate-principal-policy.
#
# Usage:
#   export AWS_PROFILE=admin-profile
#   OPERATOR_PRINCIPAL_ARN=arn:aws:iam::ACCOUNT:user/your-operator-iam-user \\
#     ./test/manual/aws-permissions-check.sh
#
# The caller must be allowed to call iam:SimulatePrincipalPolicy on OPERATOR_PRINCIPAL_ARN
# (typically an admin). Do not use the operator's own access keys for simulation unless you
# attached iam:SimulatePrincipalPolicy to that user (unusual).
#
# Without OPERATOR_PRINCIPAL_ARN, the script simulates the *current* caller (fine for ad-hoc keys).
#
# Optional overrides for resource-scoped policies:
#   COSI_TEST_USER_NAME=cosi-mynamespace-myclaim  # default: cosi-permission-smoke
#
set -euo pipefail

need_aws() {
  command -v aws >/dev/null 2>&1 || {
    echo "error: aws CLI not found" >&2
    exit 1
  }
}

eval_sim() {
  local label="$1"
  local resource_arn="$2"
  shift 2
  local out
  if ! out="$(aws iam simulate-principal-policy \
    --policy-source-arn "${POLICY_SOURCE_ARN}" \
    --resource-arns "${resource_arn}" \
    --output json \
    "$@")"; then
    echo "==> FAILED: ${label}: aws simulate-principal-policy CLI error" >&2
    echo "    If you see AccessDenied on SimulatePrincipalPolicy: run this script with an admin" >&2
    echo "    profile and set OPERATOR_PRINCIPAL_ARN to the IAM user/role ARN in your Secret." >&2
    return 1
  fi
  echo "${out}" | python3 -c '
import json, sys
data = json.load(sys.stdin)
failed = []
for ev in data.get("EvaluationResults", []):
    action = ev.get("EvalActionName", "")
    dec = ev.get("EvalDecision", "")
    if dec != "allowed":
        failed.append((action, dec, ev.get("EvalDecisionDetails")))
if failed:
    for a, d, det in sorted(failed):
        print("  " + a + ": " + d + " " + str(det or ""))
    sys.exit(1)
'
}

need_aws

IDENT="$(aws sts get-caller-identity --output json)"
CALLER_ARN="$(echo "${IDENT}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["Arn"])')"
POLICY_SOURCE_ARN="${OPERATOR_PRINCIPAL_ARN:-${CALLER_ARN}}"
echo "==> Caller (credentials): ${CALLER_ARN}"
echo "==> Simulated principal (policy-source-arn): ${POLICY_SOURCE_ARN}"

POLICY_ACCOUNT="$(echo "${POLICY_SOURCE_ARN}" | sed -n 's|^arn:aws:iam::\([0-9][0-9]*\):.*|\1|p')"
if [[ -z "${POLICY_ACCOUNT}" ]]; then
  echo "error: could not parse account id from ${POLICY_SOURCE_ARN}" >&2
  exit 1
fi

COSI_USER="${COSI_TEST_USER_NAME:-cosi-permission-smoke}"
COSI_ARN="arn:aws:iam::${POLICY_ACCOUNT}:user/${COSI_USER}"
# S3 control plane actions still need a bucket ARN context for many policies
S3_BUCKET_ARN="${S3_TEST_BUCKET_ARN:-arn:aws:s3:::k8s-s3-bucket-operator-smoke}"

S3_ACTIONS=(
  s3:CreateBucket
  s3:DeleteBucket
  s3:PutLifecycleConfiguration
  s3:ListBucket
  s3:GetBucketLocation
)

IAM_ACTIONS=(
  iam:CreateUser
  iam:DeleteUser
  iam:PutUserPolicy
  iam:DeleteUserPolicy
  iam:CreateAccessKey
  iam:DeleteAccessKey
  iam:ListAccessKeys
)

S3_ARGS=()
for a in "${S3_ACTIONS[@]}"; do S3_ARGS+=(--action-names "$a"); done
IAM_ARGS=()
for a in "${IAM_ACTIONS[@]}"; do IAM_ARGS+=(--action-names "$a"); done

echo "==> Simulating S3 actions (resource: ${S3_BUCKET_ARN})"
eval_sim "S3" "${S3_BUCKET_ARN}" "${S3_ARGS[@]}"

echo "==> Simulating IAM actions (resource: ${COSI_ARN})"
eval_sim "IAM" "${COSI_ARN}" "${IAM_ARGS[@]}"

echo "==> OK: simulate-principal-policy reports allowed for all listed actions."
echo "    Note: simulation does not catch SCPs blocking IAM, service quotas, or every condition key."
