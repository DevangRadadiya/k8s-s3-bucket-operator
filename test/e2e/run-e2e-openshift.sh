#!/bin/bash
set -e

cleanup() {
  kubectl delete ns my-app --ignore-not-found >/dev/null 2>&1 || true
  kubectl delete ns minio-ns --ignore-not-found >/dev/null 2>&1 || true
  kubectl delete ns k8s-s3-bucket-operator --ignore-not-found >/dev/null 2>&1 || true
  kubectl delete crd bucketclaims.objectstorage.k8s.io bucketclasses.objectstorage.k8s.io --ignore-not-found >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> 1. Setting up MinIO test instance"
# On strict OpenShift clusters, you may need to grant anyuid to the minio-ns default sa for the MinIO image
kubectl create ns minio-ns --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f test/e2e/minio.yaml
kubectl rollout status deployment/minio -n minio-ns --timeout=60s

echo "==> 2. Setting up k8s-s3-bucket-operator (OpenShift Profile)"
make deploy-openshift
kubectl rollout status deployment/k8s-s3-bucket-operator -n k8s-s3-bucket-operator --timeout=60s

echo "==> 3. Creating App Namespace and applying BucketClaim"
kubectl create ns my-app --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/samples/bucketclass.yaml
kubectl apply -f config/samples/bucketclaim.yaml

echo "==> 4. Waiting for BucketClaim to bind..."
sleep 3
for i in {1..10}; do
  PHASE=$(kubectl get bucketclaim my-app-images -n my-app -o jsonpath='{.status.phase}')
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
kubectl get secret my-app-images-credentials -n my-app

echo "==> 6. Verifying MinIO backend storage bucket"
kubectl exec -n minio-ns deploy/minio -- ls -ld /data/my-v120-test-bucket

echo ""
echo "✅ OpenShift End-to-End Test completed successfully! (cleanup will run automatically)"
