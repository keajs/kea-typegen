# Go Rewrite TODO

## Goal

- Keep the repo-local sample benchmark at `./bin/benchmark -c write -n 1 -w 0 --skip-prepare` with `20/20` semantic matches.
- Raise real-world semantic parity against the existing TypeScript generator on PostHog.
- Exact source equality is not required. Semantic TypeScript AST parity is the target.
- `__keaTypeGenInternal*` fields still count toward parity when the TS generator emits them.

## Current Verified Status

### Samples

- Verified on March 13, 2026:

```bash
./bin/benchmark -c write -n 1 -w 0 --skip-prepare
```

- Result:
  - semantic accuracy `20/20 (100.0%)`
  - exact accuracy `0/20 (0.0%)`
  - `TS_ONLY: 0`
  - `GO_ONLY: 0`

### PostHog

- Verified on March 13, 2026 against disposable clones under `/tmp/posthog-kea-compare.ue5vm1`.
- JS baseline:

```bash
/Users/marius/Projects/Kea/kea-typegen/bin/kea-typegen-js write -q
```

- Go compare run:

```bash
env KEA_TYPEGEN_CWD=/tmp/posthog-kea-compare.ue5vm1/go \
  GOCACHE=/tmp/kea-typegen-gocache \
  GOMODCACHE=/tmp/kea-typegen-gomodcache \
  /Users/marius/Projects/Kea/kea-typegen/rewrite/bin/kea-typegen-go \
  write -q --tsgo-bin /Users/marius/Projects/Kea/kea-typegen/.tsgo/node_modules/.bin/tsgo
```

- Semantic compare:

```bash
node ./scripts/compare-generated-typegen.js \
  --ts-dir /tmp/posthog-kea-compare.ue5vm1/js \
  --go-dir /tmp/posthog-kea-compare.ue5vm1/go \
  --top 20
```

- Result:
  - total files `739`
  - semantic matches `259`
  - semantic accuracy `35.05%`
  - exact matches `0`
  - semantic diffs `480`
  - `TS_ONLY: 0`
  - `GO_ONLY: 0`

## What Changed In This Run

- Split weak selector recovery between concrete local props and imported/loose props-backed selectors.
  - Local literal-union props selectors still recover to useful primitives in samples.
  - Imported alias-backed props selectors such as PostHog access-control `resource` / `humanReadableResource` now stay JS-compatible instead of over-recovering.
- Ignored bare-literal preferred selector returns when the inspected selector signature is still loose (`=> any` / `=> unknown`).
  - This prevents bogus public selector types like `" "` from winning over the broader JS-compatible fallback.
- Applied the same guard to type-probe recovery so props-backed primitive/alias selectors do not over-recover, while state-backed string transforms and structural `Map` / `Set` recoveries still survive.
- Fresh PostHog parity moved from `248/739 (33.56%)` to `259/739 (35.05%)`.

## Stable Coverage

- The sample benchmark is the authoritative regression bar.
- Covered sample families include:
  - object and builder `props`, `key`, `path`
  - actions, reducers, selectors, listeners, sharedListeners, events
  - loaders and lazyLoaders
  - local/default/namespace `connect(...)`
  - import ownership preservation for richer local types
  - reduced plugin surfaces for object `inline` / `form` and builder `typedForm`
  - internal selector/reducer helper fields included in semantic comparison
  - PostHog-derived canaries for builder props/key, `Map`, `Set`, and connected `Map` types

## Highest-Yield Remaining Buckets

### 1. Unresolved fallback expressions still diverge

- Representative files:
  - `frontend/src/scenes/projectLogicType.ts`
  - `frontend/src/scenes/insights/insightVizDataLogicType.ts`
  - `frontend/src/scenes/funnels/funnelDataLogicType.ts`
  - `frontend/src/layout/navigation/Breadcrumbs/breadcrumbsLogicType.ts`
- Typical mismatch:
  - JS keeps `any` or a broader union.
  - Go still lands on `null`, `undefined`, or `boolean` in some selector/value surfaces.

### 2. Imported props-backed selector parity is still weak

- Representative files:
  - `frontend/src/layout/FeaturePreviews/featurePreviewsLogicType.ts`
  - `frontend/src/layout/navigation-3000/sidepanel/panels/access_control/accessControlLogicType.ts`
  - `frontend/src/layout/navigation-3000/sidepanel/panels/access_control/roleAccessControlLogicType.ts`
  - `frontend/src/layout/navigation-3000/sidepanel/panels/access_control/ResourceAccessControlsV2/accessControlsLogicType.ts`
- Typical mismatch:
  - The worst over-recovery on props-backed primitive selectors is fixed,
  - but loader/connected surfaces in the same cluster still diverge and `featurePreviews` still differs.

### 3. Key recovery still fails in some builder-heavy files

- Representative file:
  - `frontend/src/layout/navigation-3000/sidepanel/panels/access_control/ResourceAccessControlsV2/groupedAccessControlRuleModalLogicType.ts`
- Typical mismatch:
  - JS emits `key: string`
  - Go emits `key: any`

### 4. A few files still contain summarized placeholder output

- Current count on the March 13 compare tree: `3` files with `... N more ...` placeholders.
- Representative files:
  - `frontend/src/lib/components/TaxonomicFilter/taxonomicFilterLogicType.ts`
  - `frontend/src/lib/components/TaxonomicFilter/infiniteListLogicType.ts`
  - `frontend/src/lib/utils/eventUsageLogicType.ts`

## Current Diff Concentration

- Top directory buckets from the March 13 compare:
  - `52` under `frontend/src/lib/components`
  - `27` under `frontend/src/scenes/session-recordings`
  - `23` under `frontend/src/scenes/insights`
  - `16` under `frontend/src/queries/nodes`
  - `16` under `frontend/src/scenes/data-warehouse`
  - `16` under `frontend/src/scenes/experiments`
  - `13` under `frontend/src/scenes/hog-functions`
  - `13` under `frontend/src/scenes/settings`
  - `12` under `frontend/src/layout/navigation-3000`
  - `12` under `frontend/src/scenes/data-management`

## Useful Commands

```bash
cd /Users/marius/Projects/Kea/kea-typegen
./bin/prepare-js
./bin/prepare-go
./bin/benchmark -c write -n 1 -w 0 --skip-prepare

git clone --local --no-hardlinks /Users/marius/Projects/PostHog/posthog /tmp/posthog-kea-compare.XXXXXX/js
git clone --local --no-hardlinks /Users/marius/Projects/PostHog/posthog /tmp/posthog-kea-compare.XXXXXX/go
ln -s /Users/marius/Projects/PostHog/posthog/node_modules /tmp/posthog-kea-compare.XXXXXX/js/node_modules
ln -s /Users/marius/Projects/PostHog/posthog/node_modules /tmp/posthog-kea-compare.XXXXXX/go/node_modules

cd /tmp/posthog-kea-compare.XXXXXX/js
/Users/marius/Projects/Kea/kea-typegen/bin/kea-typegen-js write -q

cd /Users/marius/Projects/Kea/kea-typegen/rewrite
env KEA_TYPEGEN_CWD=/tmp/posthog-kea-compare.XXXXXX/go \
  GOCACHE=/tmp/kea-typegen-gocache \
  GOMODCACHE=/tmp/kea-typegen-gomodcache \
  ./bin/kea-typegen-go write -q \
  --tsgo-bin /Users/marius/Projects/Kea/kea-typegen/.tsgo/node_modules/.bin/tsgo

cd /Users/marius/Projects/Kea/kea-typegen
node ./scripts/compare-generated-typegen.js \
  --ts-dir /tmp/posthog-kea-compare.XXXXXX/js \
  --go-dir /tmp/posthog-kea-compare.XXXXXX/go \
  --top 30
```

## How To Continue Next Run

1. Read this file first. Do not re-expand the old historical log.
2. Re-verify `./bin/benchmark -c write -n 1 -w 0 --skip-prepare`.
3. Recreate disposable PostHog clones and rerun the semantic compare.
4. Pick one high-yield bucket, not one file.
5. Add focused Go tests first, then patch the runtime.
6. If a fix is real-world only, distill it into a minimal `samples/` canary before finishing.
7. Replace the numbers and bullet points in this file with the new current state instead of appending another chronology section.

## Continuation Contract

- If the user says “read `GO_REWRITE_TODO.md` and continue”, use this file as the canonical handoff.
- Assume the next task is:
  - re-run the benchmark
  - re-run the fresh PostHog compare
  - fix the highest-yield remaining bucket
  - update this file with the new verified percentage
