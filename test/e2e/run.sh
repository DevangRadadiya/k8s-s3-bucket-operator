#!/bin/bash
set -euo pipefail

# Unified E2E entrypoint.
# Usage:
#   test/e2e/run.sh k8s
#   test/e2e/run.sh openshift
# Optional env:
#   OPERATOR_IMAGE=ghcr.io/<org>/<repo>:tag
#   KUBECTL=kubectl|oc

PROFILE="${1:-k8s}"

case "${PROFILE}" in
  k8s)
    exec ./test/e2e/run-e2e.sh
    ;;
  openshift)
    exec ./test/e2e/run-e2e-openshift.sh
    ;;
  aws)
    exec ./test/e2e/run-e2e-aws-localstack.sh
    ;;
  *)
    echo "Unknown profile: ${PROFILE}"
    echo "Usage: ./test/e2e/run.sh [k8s|openshift|aws]"
    exit 1
    ;;
esac

