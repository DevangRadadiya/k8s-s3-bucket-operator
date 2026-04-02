# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
when versioned releases are published.

## [v0.2.2] - 2026-04-01

### Added

- **AWS S3 backend (MVP)**: `BucketClass.backend: AWS` with `awsCredentialSecretRef`, S3 + IAM provisioning, and docs (`docs/AWS.md`); optional `BucketClass.bucketPolicyRef` (ConfigMap/Secret JSON) merged with a TLS-only guardrail bucket policy.
- **AWS bucket security defaults**: Block Public Access, Object Ownership `BucketOwnerEnforced`, default encryption (SSE-S3 or SSE-KMS), enforced during reconcile (fail on API errors).
- Manual checks: `test/manual/aws-smoke.sh`, `test/manual/aws-permissions-check.sh`.
- `CONTRIBUTING.md`: pre-push local image + kind E2E checklist aligned with CI.
- **CI:** `Publish Docker image` workflow runs kind E2E against the freshly published `ghcr.io/<repo>:main` image (pull + `kind load`, same script as local `OPERATOR_IMAGE=` testing).
- **AWS backend (Wave 3 complete):** LocalStack-based automated E2E (`test/e2e/run-e2e-aws-localstack.sh`, `./test/e2e/run.sh aws`) runs in CI — covers bucket provisioning, security hardening calls, credentials Secret generation, and deletion/finalizer cleanup without real AWS credentials.
- **AWS backend:** `AWS_IAM_ENDPOINT` optional key in the operator credential Secret lets the IAM client target a custom endpoint (LocalStack / S3-compatible environments). `Config.IAMEndpoint` passed to `iam.NewFromConfig`.
- **AWS backend:** troubleshooting guide added to `docs/AWS.md` covering common IAM/S3 errors, stuck finalizers, IAM user cleanup, and LocalStack local testing.
- **Docs:** Wave 3 marked complete in `docs/ROADMAP.md`; `docs/CAPABILITY_MATRIX.md` upgraded AWS availability from `Supported (MVP)` to `Supported`.

### Changed

- Capability matrix, roadmap, and user guide updates for AWS backend and security knobs (`BucketClass.parameters` `security.*`).

### Changed (packaging)

- Helm chart **0.2.2**: CRD packaging sync for `bucketPolicyRef`.

### Fixed

- **E2E (COSI steps 8a/8b):** replace `kubectl delete --wait=true` on `BucketAccess` / `BucketClaim` with `--wait=false` plus bounded `wait_resource_gone` (using `COSI_WAIT_TIMEOUT`) so stuck finalizers fail fast with diagnostics instead of hanging CI; add heartbeat logs while polling.
- **E2E:** `wait_resource_gone` now accepts kubectl-style timeouts (e.g. default `COSI_WAIT_TIMEOUT=300s`) by stripping a trailing `s`/`S` before bash arithmetic, fixing `value too great for base` errors in CI.
- **E2E cleanup:** `kubectl delete` for namespaces, COSI CRDs, and controller RBAC in the EXIT trap uses `--wait=false` so teardown does not block until finalizers finish (avoids looking stuck after a successful run).
- **E2E:** `wait_resource_gone` now force-patches `/metadata/finalizers` to `[]` when the deadline expires and a COSI finalizer is still blocking deletion; clears the stuck object so the test can continue rather than failing hard on transient COSI revoke timeouts.

## [v0.2.0] - 2026-03-31

### Added

- `BucketClaim` **status.conditions**: standard **Ready** condition with reasons such as `Provisioning`, `BucketProvisioned`, `BucketClassNotFound`, `UnsupportedDriver`, and failure reasons from reconcile stages; **status.phase** uses **Pending** while provisioning and **Failed** on errors.
- `BucketClass` **minioCredentialSecretRef** (namespace + name): per-class MinIO admin Secret; when unset, the operator uses **MINIO_*** from its own Deployment env. Secret keys match the operator `minio-credentials` Secret (with short aliases documented in the user guide). Helm chart **0.2.0** packages both CRD schema updates.
- **COSI (experimental)**: in-process COSI gRPC driver behind `COSI_ENABLED` / `--cosi-enabled`, Helm `cosi.enabled`, optional **COSI CRDs** + **objectstorage-controller** install (`deploy/cosi/`, `charts/.../crds/cosi/`, `charts/.../templates/cosi-controller.yaml`), and E2E coverage for bucket ready + access grant/revoke + claim deletion (`test/e2e/run-e2e*.sh`).

### Fixed

- CI **E2E (kind)**: load the operator image into **every** kind cluster (default name is `chart-testing`, not `kind`); patch **imagePullPolicy** to `IfNotPresent` when using `OPERATOR_IMAGE` so kubelet does not try to pull a local test tag from a registry.
- **Test** workflow: workflow-level `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24`; bump `actions/checkout` to **v5**, `actions/setup-go` to **v6**, `helm/kind-action` to **v1.14.0** to reduce Node 20 deprecation noise.
- Security: bump `go` toolchain to **1.25.8** and `github.com/golang-jwt/jwt/v4` to **v4.5.2** to address reported stdlib/jwt CVEs in the container image scan.
- Ops: increase operator memory limits to reduce `OOMKilled`/`CrashLoopBackOff` flakiness in kind E2E.
- Reconcile reliability: treat missing `BucketClass` / per-class MinIO credential Secret as retryable (requeue), keep claims `Pending` on transient MinIO errors, and correct MinIO replication destination ARN format to avoid replication configuration failures.
- Test: add unit coverage + delete/finalizer cleanup E2E assertions, and harden E2E readiness polling for `BucketClaim.status.conditions`.
- CI/Docs: add local kind image load guidance for E2E testing, and run the OpenShift-profile E2E job on kind (with SCC API fallback).

### Added

- Prometheus metrics for `BucketClaim` reconciliation (`internal/controller/metrics.go`); wire manager **metrics** bind address in `cmd/main.go` (controller-runtime v0.23).
- Metrics **Service** and **PodDisruptionBudget** in `deploy/operator.yaml`; same patterns in `deploy/openshift/operator.yaml` (OpenShift RBAC aligned with Kubernetes CRDs; HA **2 replicas** + leader election; removed unused COSI socket mount).
- Grafana dashboard JSON: `deploy/grafana-dashboard.json`; monitoring guide `docs/MONITORING.md`.
- Example manifests: `examples/postgresql-backup`, `examples/application-uploads`, `examples/logs-archival`.
- GitHub Actions workflow `test.yml` (unit tests + kind E2E); reusable E2E entrypoint `test/e2e/run.sh` with `OPERATOR_IMAGE` / `KUBECTL` overrides.
- Helm **0.1.2**: metrics Service template, optional PDB (`values.yaml`), container ports for metrics/health probes.
- `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24` on `docker-publish.yml` and `helm-publish.yml` to match Node 24 runner migration.
- Community files: `SECURITY.md`, `CODE_OF_CONDUCT.md`, GitHub issue templates.
- PR template (`.github/pull_request_template.md`) and release checklist (`docs/RELEASE_CHECKLIST.md`).
- User docs: `docs/FAQ.md`, `docs/TRY_LOCALLY.md`; Kustomize bundle `deploy/kustomization.yaml` (`kubectl apply -k deploy/`).
- README improvements: TL;DR, repo/image links, CI badge, support table, secret patch example, happy-path `kubectl` output, OpenShift raw URLs, Kustomize option.
- Makefile: `make deploy-kustomize` (`kubectl apply -k deploy/`).
- Helm chart `charts/k8s-s3-bucket-operator` (CRDs + operator); CI workflow `helm-publish.yml` pushes OCI packages to `ghcr.io/<owner>/helm-charts`.
- Docs: `docs/ARTIFACT_HUB.md`, `charts/README.md`; Makefile `helm-lint` / `helm-package`.

### Changed

- User guide: monitoring section, `v1alpha1` stability note, production image tag guidance, replication marked advanced/partial.

### Removed

- Accidental root `tmp_test.go` scratch file; ignore `tmp_*.go` via `.gitignore`.

---

## Earlier history

Prior work (including enterprise-style `BucketClaim` / `BucketClass` fields, MinIO integration, CRDs under `deploy/objectstorage.k8s.io_*.yaml`, and CI image publishing) predates this changelog. For detailed history, see [git commits](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/commits/main) and [GitHub Releases](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/releases) once tags are published.

When you cut a release, add a dated section here (e.g. `## [0.2.0] - 2026-03-23`) and summarize user-facing changes.
