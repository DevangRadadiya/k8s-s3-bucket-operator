# Artifact Hub — registration checklist

[Artifact Hub](https://artifacthub.io/) **does not** auto-discover charts from a random Git push. It indexes a **chart repository** you register (HTTP repo or **OCI**). This project publishes the chart to **GHCR as OCI** via [`.github/workflows/helm-publish.yml`](../.github/workflows/helm-publish.yml).

## What CI does

On push to `main` (when `charts/k8s-s3-bucket-operator/**` changes), on **releases**, or **workflow_dispatch**:

1. `helm lint` + `helm package`
2. `helm push … oci://ghcr.io/<owner>/helm-charts`

Install for users:

```bash
helm install k8s-s3-bucket-operator oci://ghcr.io/devangradadiya/helm-charts/k8s-s3-bucket-operator \
  --version <Chart.yaml version>
```

(`<owner>` must be **lowercase** for GHCR.)

## Register on Artifact Hub (one-time)

1. Sign in at [artifacthub.io](https://artifacthub.io/).
2. **Add repository** → kind **Helm charts** → **OCI** (not “Helm chart” HTTP unless you host `index.yaml`).
3. **Repository URL** (example):  
   `oci://ghcr.io/devangradadiya/helm-charts`
4. **Display name** / **URL** — point to this GitHub repo if asked.
5. If the UI asks for credentials: for **public** GHCR packages, none are needed; for **private** packages, add a robot token with `read:packages` (see [Artifact Hub private repos](https://artifacthub.io/docs/topics/repositories/#private-repositories)).

## Metadata (what “pro” charts do)

| File | Purpose |
|------|---------|
| `charts/k8s-s3-bucket-operator/Chart.yaml` | `annotations` for license, links ([Artifact Hub annotations](https://artifacthub.io/docs/topics/annotations/helm/)) |
| `charts/k8s-s3-bucket-operator/artifacthub-pkg.yml` | Extra package metadata for Artifact Hub ([docs](https://artifacthub.io/docs/topics/annotations/helm/#artifacthub-pkg-file)) |
| `charts/k8s-s3-bucket-operator/README.md` | Shown as package readme on Artifact Hub |

After registration, Artifact Hub **rescans** periodically; new chart versions appear after the next successful `helm push` and scan.

## README badge (shields.io)

If you use the [Artifact Hub repository badge](https://artifacthub.io/docs/topics/repositories/badges/), the path segment is your **Artifact Hub repository name** (the slug you chose under *Control panel → Repositories*), e.g.:

```markdown
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/k8s-s3-bucket-operator)](https://artifacthub.io/packages/search?repo=k8s-s3-bucket-operator)
```

If your repository **Name** in Artifact Hub is different (e.g. `devangradadiya-helm-charts`), replace `k8s-s3-bucket-operator` in both URLs with that slug.

## Maintainer hygiene (same as popular charts)

- Bump **`version`** in `Chart.yaml` when chart behavior changes.
- Keep **`appVersion`** aligned with the default operator image tag when you cut app releases.
- Run **`helm lint`** locally before merging (CI runs it too).
- Document breaking changes in [`CHANGELOG.md`](../CHANGELOG.md).
- Optional next steps (not required): [Helm chart-testing (`ct`)](https://github.com/helm/chart-testing), signed charts, separate `artifacthub-repo.yml` for org-level defaults.

## Troubleshooting

| Issue | Check |
|-------|--------|
| Chart not on Artifact Hub | Repository URL exact, public package on GHCR, wait for scan |
| `helm pull` 401 | Login: `helm registry login ghcr.io -u USER` |
| Wrong version | `helm search repo` / OCI tags match `Chart.yaml` `version` |
