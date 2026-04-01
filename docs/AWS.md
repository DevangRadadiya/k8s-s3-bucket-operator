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

