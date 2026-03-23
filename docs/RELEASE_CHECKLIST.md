# Release checklist

Use this checklist when cutting a tagged release (for example `v0.2.0`).

## 1) Prepare release content

- [ ] Confirm `main` is green in GitHub Actions.
- [ ] Review `CHANGELOG.md` and move important items from `Unreleased` into a new dated section:
  - `## [vX.Y.Z] - YYYY-MM-DD`
- [ ] Verify docs are current (`README.md`, `docs/USER_GUIDE.md`, samples).

## 2) Validate build and tests

- [ ] Run unit tests:

```bash
go test ./...
```

- [ ] Run e2e in your target environment:

```bash
./test/e2e/run-e2e.sh
```

- [ ] Sanity-check operator image locally if needed:

```bash
docker build -t ghcr.io/devangradadiya/k8s-s3-bucket-operator:local .
```

## 3) Tag and publish release

The `docker-publish` workflow is configured so:

- Push to `main` -> publishes `:main`
- GitHub Release `vX.Y.Z` -> publishes `:vX.Y.Z`, `:X.Y`, and `:latest`

Steps:

- [ ] Create and push an annotated tag:

```bash
git checkout main
git pull --ff-only
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

- [ ] Create the GitHub Release for that tag with release notes.

## 4) Post-release verification

- [ ] Confirm workflow succeeded and images exist in GHCR.
- [ ] Pull and inspect expected tags:

```bash
docker pull ghcr.io/devangradadiya/k8s-s3-bucket-operator:vX.Y.Z
docker pull ghcr.io/devangradadiya/k8s-s3-bucket-operator:latest
```

- [ ] Validate deployment manifests still reference the intended default tag.
- [ ] Announce release notes (if applicable).

## 5) Rollback plan

- [ ] Keep prior stable tag documented.
- [ ] If regression found, redeploy previous image tag and open a hotfix PR.
