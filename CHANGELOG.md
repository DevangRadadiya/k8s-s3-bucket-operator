# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
when versioned releases are published.

## [Unreleased]

### Added

- `BucketClaim` **status.conditions**: standard **Ready** condition with reasons such as `Provisioning`, `BucketProvisioned`, `BucketClassNotFound`, `UnsupportedDriver`, and failure reasons from reconcile stages; **status.phase** uses **Pending** while provisioning and **Failed** on errors.
- `BucketClass` **minioCredentialSecretRef** (namespace + name): per-class MinIO admin Secret; when unset, the operator uses **MINIO_*** from its own Deployment env. Secret keys match the operator `minio-credentials` Secret (with short aliases documented in the user guide). Helm chart **0.1.4** packages both CRD schema updates.

### Fixed

- CI **E2E (kind)**: load the operator image into **every** kind cluster (default name is `chart-testing`, not `kind`); patch **imagePullPolicy** to `IfNotPresent` when using `OPERATOR_IMAGE` so kubelet does not try to pull a local test tag from a registry.
- **Test** workflow: workflow-level `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24`; bump `actions/checkout` to **v5**, `actions/setup-go` to **v6**, `helm/kind-action` to **v1.14.0** to reduce Node 20 deprecation noise.

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
