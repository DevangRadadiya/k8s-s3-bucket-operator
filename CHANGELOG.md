# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
when versioned releases are published.

## [Unreleased]

### Added

- Community files: `SECURITY.md`, `CODE_OF_CONDUCT.md`, GitHub issue templates.
- PR template (`.github/pull_request_template.md`) and release checklist (`docs/RELEASE_CHECKLIST.md`).
- User docs: `docs/FAQ.md`, `docs/TRY_LOCALLY.md`; Kustomize bundle `deploy/kustomization.yaml` (`kubectl apply -k deploy/`).
- README improvements: TL;DR, repo/image links, CI badge, support table, secret patch example, happy-path `kubectl` output, OpenShift raw URLs, Kustomize option.
- Makefile: `make deploy-kustomize` (`kubectl apply -k deploy/`).

---

## Earlier history

Prior work (including enterprise-style `BucketClaim` / `BucketClass` fields, MinIO integration, CRDs under `deploy/objectstorage.k8s.io_*.yaml`, and CI image publishing) predates this changelog. For detailed history, see [git commits](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/commits/main) and [GitHub Releases](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/releases) once tags are published.

When you cut a release, add a dated section here (e.g. `## [0.2.0] - 2026-03-23`) and summarize user-facing changes.
