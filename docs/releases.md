# Releasing kea-typegen and kea-forms

In the repository you want to release, open **Actions → Release → Run workflow** on `main`. Leave the version empty for the next minor, or enter a higher semantic version such as `3.10.1` or `4.0.0-beta.1`. Stable versions publish under `latest` and prereleases under `next`.

The workflow prepares the version and changelog in its checkout. It takes changelog entries from commits since the previous version, including repositories whose last npm release has no Git tag. It builds and tests the package before pushing release metadata or publishing anything.

For typegen, CI takes the list of OS and CPU pairs from the pinned TypeScript compiler's npm manifest. It cross-compiles a worker per pair and packs each as `@keajs/kea-typegen-<platform>-<arch>`. The main tarball declares those packages as exact-version optional dependencies. Linux, Windows and macOS jobs on x64 and arm64 install the actual tarballs with lifecycle scripts disabled, generate a typed action, and check that a second run is clean. The release also tags the first public v1 Go plugin SDK at `plugin/v1.0.0`.

For forms, CI builds the Go visitor once for WASI and includes it in the regular `kea-forms` tarball alongside the JavaScript visitor. Its release workflow calls the shared workflow in `keajs/kea-typegen`. Release typegen first. The forms installation test exercises its packed visitor with the published typegen CLI.

## Registry and repository setup

The publishing identity needs access to `kea-typegen`, `kea-forms` and the `@keajs` npm scope for the worker packages. Configure direct npm trusted publishing for the corresponding **calling** repository and workflow filename `release.yml`. For initial creation of worker packages, the workflow also accepts a GitHub Actions secret named `NPM_TOKEN` with npm publish access to that scope. The workflow's GitHub token needs permission to push the release commit to `main`, create tags, and create a release under the repository's branch and tag rules. [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/)

## Interrupted releases

The publish job verifies registry integrity for existing versions, pushes the changelog commit and tags atomically, publishes all platform packages, then publishes the main package and GitHub release. If publication is interrupted, use **Re-run failed jobs** on the same run. This reuses the tested tarballs and skips versions whose SHA-512 integrity already matches. A different archive at an existing version stops the release. If `main` moved before the release commit landed, dispatch a fresh workflow on the new revision.
