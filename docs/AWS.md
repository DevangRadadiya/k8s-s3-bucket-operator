# AWS backend (S3 + IAM)

This operator supports an AWS backend via `BucketClass.backend: AWS`.

## What gets created in AWS

For each `BucketClaim`:

- An S3 bucket (name is the claim-derived bucket name; see `BucketClaim.spec.bucketName` and fallback naming).
- An IAM User named like `cosi-<normalized accountID>` (accountID is `<namespace>-<claimName>`).
- An inline IAM policy attached to that user, scoped to the bucket ARN(s).
- An IAM access key for that user (unless the operator detects and reuses existing keys in the credentials Secret).

## Bucket security defaults (AWS backend)

When `backend: AWS`, the operator applies security hardening after bucket creation (and will fail reconcile if AWS rejects it):

- **Block public access**: enables S3 Block Public Access (all 4 flags).
- **Disable ACLs**: sets Object Ownership to `BucketOwnerEnforced`.
- **Default encryption**: enables SSE-S3 by default.
- **TLS-only bucket policy**: attaches a bucket policy that denies non-TLS requests (`aws:SecureTransport=false`).

You can override behavior via `BucketClass.parameters`:

- `security.blockPublicAccess`: `true|false` (default `true`)
- `security.disableAcls`: `true|false` (default `true`)
- `security.defaultEncryption`: `SSE-S3|SSE-KMS|none` (default `SSE-S3`)
- `security.kmsKeyArn`: KMS key ARN (required when `security.defaultEncryption=SSE-KMS`)
- `security.tlsOnlyPolicy`: `true|false` (default `true`)

### Optional custom bucket policy (bucketPolicyRef)

You may provide an additional bucket policy JSON document via `bucketPolicyRef` (ConfigMap or Secret). The operator will **merge** it with the built-in TLS-only guardrail (built-in statements first, then your statements).

## Required Kubernetes config

### 1) Create an AWS credentials Secret

Create a Secret referenced by `BucketClass.awsCredentialSecretRef`:

- Required keys:
  - `AWS_REGION`
  - `AWS_ACCESS_KEY_ID`
  - `AWS_SECRET_ACCESS_KEY`
- Optional keys:
  - `AWS_S3_ENDPOINT` (custom S3 endpoint for testing / S3-compatible implementations)

Example:

```bash
kubectl create ns backend-creds --dry-run=client -o yaml | kubectl apply -f -

kubectl -n backend-creds create secret generic aws-creds \
  --from-literal=AWS_REGION="us-east-1" \
  --from-literal=AWS_ACCESS_KEY_ID="REDACTED" \
  --from-literal=AWS_SECRET_ACCESS_KEY="REDACTED" \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 2) Create an AWS BucketClass

```yaml
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: aws-standard
driverName: k8s-s3-bucket-operator
backend: AWS
deletionPolicy: Delete
parameters:
  region: "us-east-1"
  security.defaultEncryption: "SSE-S3"
  security.tlsOnlyPolicy: "true"
awsCredentialSecretRef:
  namespace: backend-creds
  name: aws-creds
```

Example with a policy reference:

```yaml
bucketPolicyRef:
  kind: ConfigMap
  namespace: backend-creds
  name: aws-bucket-policy
  key: policy.json
```

## Real AWS smoke test (recommended)

This is a real, end-to-end test against AWS. It does **not** require modifying the operator image.

### Step 0: prerequisites

- Operator installed in your cluster
- `kubectl` configured to the target cluster
- AWS CLI configured locally (`aws sts get-caller-identity` works)

### Step 1: create a dedicated IAM user for the operator

The operator needs AWS credentials to:

- create/delete S3 buckets
- create/delete IAM users
- attach/delete inline policies
- create/delete IAM access keys

Create an IAM user:

```bash
export OP_PREFIX="k8s-s3-bucket-operator"
aws iam create-user --user-name "${OP_PREFIX}-operator"
aws iam create-access-key --user-name "${OP_PREFIX}-operator"
```

Attach a minimal inline policy.

If you want strict bucket scoping, set `BUCKET_PREFIX` and replace the `"Resource": "*"` in the S3 statement with
`"arn:aws:s3:::${BUCKET_PREFIX}*"` and `"arn:aws:s3:::${BUCKET_PREFIX}*/*"`.

```bash
cat > /tmp/k8s-s3-bucket-operator-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "S3Buckets",
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:DeleteBucket",
        "s3:PutLifecycleConfiguration",
        "s3:ListBucket",
        "s3:GetBucketLocation",
        "s3:PutPublicAccessBlock",
        "s3:PutBucketOwnershipControls",
        "s3:PutBucketEncryption",
        "s3:PutBucketPolicy"
      ],
      "Resource": "*"
    },
    {
      "Sid": "IAMPerClaimUsers",
      "Effect": "Allow",
      "Action": [
        "iam:CreateUser",
        "iam:DeleteUser",
        "iam:PutUserPolicy",
        "iam:DeleteUserPolicy",
        "iam:CreateAccessKey",
        "iam:DeleteAccessKey",
        "iam:ListAccessKeys"
      ],
      "Resource": "arn:aws:iam::*:user/cosi-*"
    }
  ]
}
EOF

aws iam put-user-policy \
  --user-name "${OP_PREFIX}-operator" \
  --policy-name "${OP_PREFIX}-operator" \
  --policy-document file:///tmp/k8s-s3-bucket-operator-policy.json
```

Optional **read-only** actions on the same bucket ARNs (for `aws s3api get-*` verification from the operator principal or an auditor role; not required for provisioning):

- `s3:GetBucketPublicAccessBlock`
- `s3:GetBucketOwnershipControls`
- `s3:GetEncryptionConfiguration`
- `s3:GetBucketPolicy`

### Step 2: store those operator AWS keys in Kubernetes

Use the access key created in Step 1 as the values in `aws-creds` Secret (see above), then apply the `BucketClass`.

### Step 3: create a BucketClaim and wait for Ready

```bash
kubectl create ns aws-test --dry-run=client -o yaml | kubectl apply -f -

cat <<'EOF' | kubectl -n aws-test apply -f -
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: claim1
spec:
  bucketClassName: aws-standard
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
```

Check status + generated Secret:

```bash
kubectl -n aws-test get bucketclaim claim1 -o yaml
kubectl -n aws-test get secret claim1-credentials -o yaml
```

### Step 4: verify in AWS

Get the bucket name the operator chose:

```bash
BUCKET="$(kubectl -n aws-test get secret claim1-credentials -o jsonpath='{.data.bucketName}' | base64 -d)"
echo "$BUCKET"
```

Confirm it exists:

```bash
aws s3api head-bucket --bucket "$BUCKET"
```

### Step 5: delete the claim and verify cleanup

```bash
kubectl -n aws-test delete bucketclaim claim1
aws s3api head-bucket --bucket "$BUCKET" && echo "still exists" || echo "deleted"
```

## Known limitations (current)

- `BucketClaim.spec.quota`: **no-op** for AWS
- `BucketClaim.spec.replicationTarget`: **no-op** for AWS (planned)

## Troubleshooting

### BucketClaim stays in Pending / Failed

1. Check operator logs:
   ```bash
   kubectl -n k8s-s3-bucket-operator logs deploy/k8s-s3-bucket-operator --tail=60
   ```
2. Check the BucketClaim status conditions for an error message:
   ```bash
   kubectl -n <namespace> get bucketclaim <name> -o yaml | grep -A 20 conditions
   ```
3. Common causes and fixes:

| Symptom | Cause | Fix |
|---------|-------|-----|
| `secret … not found` | `awsCredentialSecretRef` namespace/name wrong, or Secret not created | Verify Secret name and namespace match the `BucketClass.awsCredentialSecretRef` |
| `secret must define AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY` | Missing keys in the operator Secret | Add the missing keys |
| `operation error S3: CreateBucket, … AccessDenied` | Operator IAM user missing `s3:CreateBucket` | Attach the IAM policy from Step 1 in this doc |
| `operation error IAM: CreateUser, … AccessDenied` | Operator IAM user missing `iam:CreateUser` on `arn:…:user/cosi-*` | Check the IAM policy `iam:CreateUser` resource scope |
| `operation error S3: PutPublicAccessBlock, … AccessDenied` | Operator IAM user missing hardening permissions | Add `s3:PutPublicAccessBlock`, `s3:PutBucketOwnershipControls`, `s3:PutBucketEncryption`, `s3:PutBucketPolicy` to the IAM policy |
| `security.kmsKeyArn is required when security.defaultEncryption=SSE-KMS` | `BucketClass.parameters` has `security.defaultEncryption=SSE-KMS` but no KMS key ARN | Add `security.kmsKeyArn: <arn>` to `BucketClass.parameters` |
| `bucketPolicyRef JSON invalid` | The ConfigMap/Secret referenced by `bucketPolicyRef` contains malformed JSON | Validate with `python3 -c "import json; json.load(open('policy.json'))"` |
| `LimitExceeded … AccessKeysPerUser` | The per-claim IAM user hit the 2-key AWS quota; operator auto-rotates but may fail if a previous run left stale keys | Operator handles this automatically; if repeated, manually delete old access keys for the `cosi-*` user |

### Bucket not deleted after claim deletion

If `deletionPolicy: Delete` is set but the bucket persists after `kubectl delete bucketclaim`:

1. Check the operator logs — a failed `DeleteBucket` call will appear as an error.
2. Check whether the bucket still has objects. AWS does not allow deleting non-empty buckets. The operator does **not** empty buckets automatically; you must delete objects first:
   ```bash
   aws s3 rm s3://<bucket-name> --recursive
   kubectl delete bucketclaim <name> -n <namespace>
   ```
3. Check if the BucketClaim finalizer is stuck:
   ```bash
   kubectl -n <namespace> get bucketclaim <name> -o jsonpath='{.metadata.finalizers}'
   ```
   If the operator is down, the finalizer will never run. Restart the operator and try again.

### IAM user not cleaned up

After `kubectl delete bucketclaim`, the `cosi-*` IAM user should be deleted. If it is not:

1. Check operator logs for `RevokeAccess` / `DeleteUser` errors.
2. If the user has an attached managed policy (not inline), the operator cannot delete it. The operator only manages the inline policy it created. Detach any manually-attached managed policies first.
3. Manual cleanup:
   ```bash
   USER=cosi-<namespace>-<claimname>
   aws iam list-user-policies --user-name "$USER"
   aws iam delete-user-policy --user-name "$USER" --policy-name <policy-name>
   aws iam list-access-keys --user-name "$USER"
   aws iam delete-access-key --user-name "$USER" --access-key-id <id>
   aws iam delete-user --user-name "$USER"
   ```

### Testing with LocalStack (no real AWS account)

The operator supports `AWS_IAM_ENDPOINT` and `AWS_S3_ENDPOINT` overrides in the credentials Secret, so you can point both S3 and IAM at a [LocalStack](https://localstack.cloud) instance:

```bash
kubectl create secret generic localstack-creds \
  --from-literal=AWS_REGION=us-east-1 \
  --from-literal=AWS_ACCESS_KEY_ID=test \
  --from-literal=AWS_SECRET_ACCESS_KEY=test \
  --from-literal=AWS_S3_ENDPOINT=http://localstack:4566 \
  --from-literal=AWS_IAM_ENDPOINT=http://localstack:4566
```

The automated LocalStack E2E (`./test/e2e/run.sh aws`) uses this approach in CI.

