# Public Repository Management Guide

As you prepare to make the `k8s-s3-bucket-operator` public on GitHub, you should implement strong safeguards to protect the codebase and facilitate a smooth open-source contribution process.

## 1. Branch Protection Rules

To ensure that only approved and tested code merges into your `main` branch, you must enable Branch Protection Rules. This guarantees that your public source of truth is always deployable.

1. Navigate to your GitHub Repository -> **Settings** -> **Branches**.
2. Under "Branch protection rules", click **Add rule**.
3. **Branch name pattern**: `main`
4. **Protect matching branches**:
   - Check **Require a pull request before merging**.
   - Check **Require approvals** (Set to at least 1).
   - Check **Require status checks to pass before merging** (Useful if you attach the `run-e2e.sh` script to GitHub Actions).
   - Check **Do not allow bypassing the above settings**.
5. Click **Create** to save.

## 2. GitHub Releases

Creating a Release provides users a snapshot of your operator version that is stable and production-ready. 

### Creating a Release via the GitHub UI:
1. On the main page of your repository, click **Releases** on the right side.
2. Click **Draft a new release**.
3. Choose a tag or create a new one (e.g., `v1.0.0`).
4. Set the **Release title** (e.g., `v1.0.0 - Production Ready Pivot`) and auto-generate or manually type the Release notes outlining changes.
5. Click **Publish release**.

### Creating a Release via the CLI:
1. Install the GitHub CLI (`gh`).
2. Run standard git tagging:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
3. Use the CLI to draft the release using documentation from your commits:
   ```bash
   gh release create v1.0.0 --title "v1.0.0" --notes "Operator rewrite: eliminated COSI sidecars, natively integrated OpenShift SCCs, unified credentials injection."
   ```
