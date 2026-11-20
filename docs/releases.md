# Releasing kea-typegen, kea-forms and kea-saga

In the repository you want to release, open **Actions → Release → Run workflow** on the default branch. A version already prepared in both `package.json` and the first `## <version> - Unreleased` changelog entry is dated and released as written. Otherwise, leaving the input empty selects the next minor. You can also enter the desired version. Stable versions publish under `latest` and prereleases under `next`.

The workflow prepares the version and changelog in its checkout. It takes changelog entries from commits since the previous version, including repositories whose last npm release has no Git tag. It builds and tests the package before pushing release metadata or publishing anything.

For typegen, CI takes the list of OS and CPU pairs from the pinned TypeScript compiler's npm manifest. It cross-compiles a worker per pair and packs each as `@keajs/kea-typegen-<platform>-<arch>`. The main tarball declares those packages as exact-version optional dependencies. Linux, Windows and macOS jobs on x64 and arm64 install the actual tarballs with lifecycle scripts disabled, generate a typed action, and check that a second run is clean. The release also tags the logic-field SDK at `plugin/v1.1.0`.

For forms and saga, CI builds one WASI module per package and ships it alongside the JavaScript visitor. The same native workers run both modules on every platform. Each plugin installs its own packed tarball with the published typegen CLI and checks the generated declaration before publication.

## Configure npm and publish

From a signed-in npm maintainer session, configure the trusted publisher once for each package that still needs it. Each command opens npm's authentication flow.

```sh
npm trust github kea-forms --repo=keajs/kea-forms --file=release.yml --allow-publish
npm trust github kea-saga --repo=keajs/kea-saga --file=release.yml --allow-publish
```

In either plugin checkout, `flox activate` and `npm run release:setup` configure that package. To check an existing configuration, run `npm trust list <package>`.

Forms can release against typegen 4.0.0. Before releasing saga, publish a typegen version with logic-field support and wait for its worker packages to become available on npm. Then use each repository's default branch:

```sh
gh workflow run release.yml --repo keajs/kea-forms --ref main -f version=3.3.0
gh workflow run release.yml --repo keajs/kea-saga --ref master -f version=3.2.0
```

If a future compiler adds a platform, create its `@keajs/kea-typegen-<platform>-<arch>` package on npm and grant `keajs/kea-typegen`'s `release.yml` publish access before releasing. Existing workers reuse their trusted publisher. [npm staged package creation](https://github.blog/changelog/2026-10-02-npm-staged-publishing-now-supports-creating-new-packages/) · [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/)

The workflow's GitHub token needs permission to push the release commit to the default branch, create tags, and create a GitHub release under the repository's branch and tag rules.

## Interrupted releases

The publish job verifies registry integrity for existing versions, pushes the changelog commit and tags atomically, publishes all platform packages, then publishes the main package and GitHub release. If publication is interrupted, use **Re-run failed jobs** on the same run. This reuses the tested tarballs and skips versions whose SHA-512 integrity already matches. A different archive at an existing version stops the release. If the default branch moved before the release commit landed, dispatch a fresh workflow on the new revision.
