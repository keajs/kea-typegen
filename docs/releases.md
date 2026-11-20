# Releasing kea-typegen and kea-forms

In the repository you want to release, open **Actions → Release → Run workflow** on `main`. A version already prepared in both `package.json` and the first `## <version> - Unreleased` changelog entry is dated and released as written. Otherwise, leaving the input empty selects the next minor. You can also enter the desired version. Stable versions publish under `latest` and prereleases under `next`.

The workflow prepares the version and changelog in its checkout. It takes changelog entries from commits since the previous version, including repositories whose last npm release has no Git tag. It builds and tests the package before pushing release metadata or publishing anything.

For typegen, CI takes the list of OS and CPU pairs from the pinned TypeScript compiler's npm manifest. It cross-compiles a worker per pair and packs each as `@keajs/kea-typegen-<platform>-<arch>`. The main tarball declares those packages as exact-version optional dependencies. Linux, Windows and macOS jobs on x64 and arm64 install the actual tarballs with lifecycle scripts disabled, generate a typed action, and check that a second run is clean. The release also tags the first public v1 Go plugin SDK at `plugin/v1.0.0`.

For forms, CI builds the Go visitor once for WASI and includes it in the regular `kea-forms` tarball alongside the JavaScript visitor. Its release workflow calls the shared workflow in `keajs/kea-typegen`. Release typegen first. The forms installation test exercises its packed visitor with the published typegen CLI.

## First release with native workers

Use a local npm account with 2FA, publish access to `kea-typegen`, and permission to create public packages under the **npm** scope `@keajs`. From a dependency-installed typegen checkout on `main`, run:

```sh
npx --yes npm@11.21.3 login --registry=https://registry.npmjs.org
npx --yes npm@11.21.3 run release:setup
gh workflow run release.yml --repo keajs/kea-typegen --ref main -f version=4.0.0
```

The setup command lists its targets with `-- --list`. It stages a metadata-only bootstrap version for each missing worker, then grants `keajs/kea-typegen`'s `release.yml` direct publish access to typegen and its workers. npm creates a public placeholder for each new name as soon as it is staged. Leave the bootstrap stages pending or reject them after setup. The release workflow builds and publishes the actual binaries and the main package. Complete the first trusted publish within 48 hours of creating trust. [npm staged package creation](https://github.blog/changelog/2026-10-02-npm-staged-publishing-now-supports-creating-new-packages/) · [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/)

After typegen 4.0.0 is available on npm, configure and release forms:

```sh
npx --yes npm@11.21.3 trust github kea-forms --repo=keajs/kea-forms --file=release.yml --allow-publish
gh workflow run release.yml --repo keajs/kea-forms --ref main -f version=3.3.0
```

If a matching trusted publisher already exists, use it. The forms caller forwards the GitHub permissions needed by the shared release workflow. In both repositories, the workflow's GitHub token needs permission to push the release commit to `main`, create tags, and create a GitHub release under the repository's branch and tag rules.

## Interrupted releases

The publish job verifies registry integrity for existing versions, pushes the changelog commit and tags atomically, publishes all platform packages, then publishes the main package and GitHub release. If publication is interrupted, use **Re-run failed jobs** on the same run. This reuses the tested tarballs and skips versions whose SHA-512 integrity already matches. A different archive at an existing version stops the release. If `main` moved before the release commit landed, dispatch a fresh workflow on the new revision.
