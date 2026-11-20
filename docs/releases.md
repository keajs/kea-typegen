# Releasing kea-typegen and kea-forms

In the repository you want to release, open **Actions → Release → Run workflow** on `main`. A version already prepared in both `package.json` and the first `## <version> - Unreleased` changelog entry is dated and released as written. Otherwise, leaving the input empty selects the next minor. You can also enter the desired version. Stable versions publish under `latest` and prereleases under `next`.

The workflow prepares the version and changelog in its checkout. It takes changelog entries from commits since the previous version, including repositories whose last npm release has no Git tag. It builds and tests the package before pushing release metadata or publishing anything.

For typegen, CI takes the list of OS and CPU pairs from the pinned TypeScript compiler's npm manifest. It cross-compiles a worker per pair and packs each as `@keajs/kea-typegen-<platform>-<arch>`. The main tarball declares those packages as exact-version optional dependencies. Linux, Windows and macOS jobs on x64 and arm64 install the actual tarballs with lifecycle scripts disabled, generate a typed action, and check that a second run is clean. The release also tags the first public v1 Go plugin SDK at `plugin/v1.0.0`.

For forms, CI builds the Go visitor once for WASI and includes it in the regular `kea-forms` tarball alongside the JavaScript visitor. Its release workflow calls the shared workflow in `keajs/kea-typegen`. Release typegen first. The forms installation test exercises its packed visitor with the published typegen CLI.

## Publishing the prepared versions

Run the typegen release from `main`. GitHub Actions uses trusted publishing to publish the 21 native workers first and `kea-typegen` last.

```sh
gh workflow run release.yml --repo keajs/kea-typegen --ref main -f version=4.0.0
```

After `kea-typegen@4.0.0` is available under `latest`, run the forms release. If the forms publisher still needs configuring, a maintainer can run `npm trust github kea-forms --repo=keajs/kea-forms --file=release.yml --allow-publish` from a signed-in npm session.

```sh
gh workflow run release.yml --repo keajs/kea-forms --ref main -f version=3.3.0
```

If a future compiler adds a platform, create its `@keajs/kea-typegen-<platform>-<arch>` package on npm and grant `keajs/kea-typegen`'s `release.yml` publish access before releasing. Existing workers reuse their trusted publisher. [npm staged package creation](https://github.blog/changelog/2026-10-02-npm-staged-publishing-now-supports-creating-new-packages/) · [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/)

The workflow's GitHub token needs permission to push the release commit to `main`, create tags, and create a GitHub release under the repository's branch and tag rules.

## Interrupted releases

The publish job verifies registry integrity for existing versions, pushes the changelog commit and tags atomically, publishes all platform packages, then publishes the main package and GitHub release. If publication is interrupted, use **Re-run failed jobs** on the same run. This reuses the tested tarballs and skips versions whose SHA-512 integrity already matches. A different archive at an existing version stops the release. If `main` moved before the release commit landed, dispatch a fresh workflow on the new revision.
