# COSI mode (experimental)

This operator can run an **in-process COSI gRPC driver** (Unix socket) alongside the existing Wave 1 controller.

Important notes:

- The Go module import path is still `cosi.v1alpha1` (see `sigs.k8s.io/container-object-storage-interface-spec`), even though the Kubernetes API group is `objectstorage.k8s.io`.
- Wave 1 remains the default: COSI is **off** unless explicitly enabled.

## What gets installed

1. **Additional CRDs** (cluster-scoped unless noted):
   - `buckets.objectstorage.k8s.io`
   - `bucketaccesses.objectstorage.k8s.io` (namespaced)
   - `bucketaccessclasses.objectstorage.k8s.io`
2. **COSI controller** (`objectstorage-controller`) which watches `BucketClaim` and creates `Bucket` objects.
3. **Provisioner sidecar** (`objectstorage-sidecar`) colocated in the operator pod, connecting to the driver socket.

Helm chart sources:

- CRDs: `charts/k8s-s3-bucket-operator/crds/cosi/*.yaml`
- Controller + RBAC: `charts/k8s-s3-bucket-operator/templates/cosi-controller.yaml`
- Sidecar wiring: `charts/k8s-s3-bucket-operator/templates/deployment.yaml`

Raw YAML kustomize bundle:

- `deploy/cosi/` (`kubectl apply -k deploy/cosi`)

## Enabling COSI

### Helm

Set:

- `cosi.enabled=true`

This will:

- mount a shared `emptyDir` at `/var/lib/cosi`
- set operator env:
  - `COSI_ENABLED=true`
  - `COSI_DRIVER_NAME` (defaults to `k8s-s3-bucket-operator`)
  - `COSI_SOCKET_PATH` (defaults to `/var/lib/cosi/cosi.sock`)
- add the `objectstorage-sidecar` container
- install the COSI controller (if `cosi.controller.enabled=true`)

### Raw YAML / manual

You need the same ingredients as Helm:

- apply COSI CRDs + controller manifests (`deploy/cosi/`)
- patch the operator `Deployment` to add env + volume + sidecar (see `test/e2e/run-e2e.sh` and `test/e2e/run-e2e-openshift.sh` for reference patches)

## Driver matching rules

`BucketClass.driverName` and `BucketAccessClass.driverName` must match:

- Helm: `.Values.cosi.driverName`
- env: `COSI_DRIVER_NAME`
- `BucketAccessClass.authenticationType` must be `Key` (this operator currently supports only COSI key-based credentials)

## Passing “enterprise” knobs through COSI `BucketClass.parameters`

COSI’s `Bucket` object copies `BucketClass.parameters` into `Bucket.spec.parameters`.

This driver understands optional keys:

- `bucketName`: force the backend bucket name (recommended; COSI-generated `Bucket` object names include uppercase UUID segments)
- `quota`, `accessType`, `lifecycleRules` (JSON array), `replicationTarget` (JSON object) — see `provisioning.ApplyClaimParameterExtensions`

## Known limitations

- The upstream provisioner sidecar currently **hardcodes** some fields when minting the `BucketInfo` secret (endpoint/region). Clients should treat COSI secrets as **compatible with the COSI API**, not necessarily identical to Wave 1’s `<claim>-credentials` Secret shape.

## Which mode should I use?

**Standalone mode (default, non-COSI)** — use this when:

- You control the platform and just want a simple `BucketClaim`/`BucketClass` API for MinIO.
- You don’t need to integrate with other COSI-aware components or a broader platform standard.
- You want the Wave 1 Secret shape (`<claim>-credentials`) and behavior exactly as documented in the main user guide.

**COSI mode (experimental hybrid)** — consider this when:

- Your cluster/platform is standardizing on **COSI** and you want this operator to participate in that ecosystem.
- Other teams expect to interact with **COSI `Bucket` / `BucketAccess`** resources rather than the standalone CRDs only.
- You’re comfortable with “beta” level support and can follow release notes for breaking changes.

Both modes share the same MinIO backend logic; enabling COSI mode does **not** remove or change the standalone CRDs.

## Migration examples (high level)

### Standalone → COSI hybrid (same MinIO backend)

1. **Prepare**:
   - Ensure you can tolerate a short maintenance window (no bucket deletions should occur, but APIs will roll).
   - Confirm your MinIO credentials and endpoints are correct and documented.
2. **Install COSI components**:
   - Apply `deploy/cosi/` (or enable `.Values.cosi.enabled=true` in Helm).
   - Confirm the `objectstorage-controller` Deployment is healthy.
3. **Enable the driver**:
   - Set `COSI_ENABLED=true` (Helm: `.Values.cosi.enabled=true`) and ensure the sidecar + socket volume are present.
4. **Introduce COSI BucketClasses / BucketAccessClasses**:
   - Create COSI `BucketClass` / `BucketAccessClass` objects that map to your existing MinIO configuration.
   - Ensure `driverName` matches the operator's `COSI_DRIVER_NAME` (Helm `.Values.cosi.driverName`).
   - Ensure `BucketAccessClass.authenticationType: Key`.
5. **Migrate workloads gradually**:
   - New apps: use COSI `BucketClaim` + `BucketAccess`.
   - Existing apps: keep using standalone `BucketClaim` until you are ready to migrate and test them under COSI.

### COSI → Standalone (rollback)

If you need to revert to pure standalone mode:

1. **Stop new COSI usage**: stop creating new COSI `BucketClaim` / `BucketAccess` objects.
2. **Drain running COSI access**: ensure no workloads depend solely on COSI-generated `BucketInfo` Secrets.
3. **Disable driver + sidecar**:
   - Set `COSI_ENABLED=false` (or revert Helm values) and remove the COSI sidecar/volume from the operator.
4. **Optionally remove COSI CRDs** once no COSI resources remain and you no longer need the mode.

## What the COSI E2E currently validates

The COSI E2E scripts (`test/e2e/run-e2e*.sh`) run in a local cluster and validate that:

- A COSI `BucketClaim` eventually results in:
  - a `Bucket` with `status.bucketReady=true`, and
  - a `BucketAccess` with `status.accessGranted=true`.
- The operator can create and update:
  - the backing MinIO bucket (reusing Wave 1 provisioning logic), and
  - a COSI-compatible credentials Secret containing `BucketInfo` JSON.
- Deletion flows behave as expected:
  - `BucketAccess` deletion revokes access but does **not** delete the backend bucket.
  - `BucketClaim` deletion respects the `BucketClass.deletionPolicy`.

## Validation checklist (what to check after migration)

- `BucketClaim.status.phase` becomes `Bound`
- `BucketClaim.status.conditions[type=Ready].status` becomes `True` with reason `BucketProvisioned`
- `BucketAccess.status.accessGranted` becomes `true`
- The credentials `Secret` referenced by `BucketAccess.spec.credentialsSecretName` contains `BucketInfo` JSON under `.data.BucketInfo`
- Deleting `BucketAccess` clears the MinIO user created for that access (but keeps the backend bucket directory)
- Deleting `BucketClaim` deletes (or retains) the backend bucket depending on `BucketClass.deletionPolicy`
