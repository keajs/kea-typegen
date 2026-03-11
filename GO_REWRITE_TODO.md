# Go Rewrite TODO

## Target

- The working goal is now: both the Go rewrite and the existing TypeScript implementation should be able to generate types for every logic-bearing file under `samples/`.
- The concrete acceptance check is now `./bin/benchmark -c write -n 1 -w 1`, and the target is `16/16` semantic matches in that benchmark.
- Byte-for-byte output equality is not required. The target is semantic parity, with emitted TypeScript ASTs staying as close as practical between the Go and TS versions.
- `__keaTypeGenInternal*` helper fields are part of that parity target again. They matter for selector/reducer helper fidelity and are now counted by the in-repo semantic benchmark.
- Plugin parity is not deferred when the current TypeScript generator emits plugin-produced public or internal fields in the sample corpus. If a sampled plugin shape affects benchmark parity, it is in scope.

## Current state

- `rewrite/` is now the start of the real Go rewrite, separate from the one-off PoCs under `poc/`.
- The CLI at [`rewrite/cmd/kea-typegen-go/main.go`](/Users/marius/Projects/Kea/kea-typegen/rewrite/cmd/kea-typegen-go/main.go) still opens TypeScript projects through the `typescript-go` API, and it now has two paths:
  - native `check`, `write`, and polling `watch` subcommands with `.kearc`/`tsconfig.json` discovery, root/types path resolution, cache restore/writeback, delete support, single-file mode, generated `*Type.ts` writes, object-path insertion, and local logic type import / `kea<LogicType>` source repairs
  - native `--convert-to-builders` rewrites for object-style logics, including supported builder import insertion and reuse of existing builder aliases such as `path as logicPath`
  - `-format report`: the raw inspection JSON
  - `-format model`: a Go-native normalized logic model
  - `-format typegen`: a reduced `.typegen`-style TypeScript interface
- The scanner is source-text based. It now handles both `kea({ ... })` and `kea([ ... ])` builder syntax, records the input kind, recognizes quoted/computed string top-level section keys such as `['actions']`, canonicalizes aliased builder imports such as `path as logicPath` back to their logical section names, and understands namespace-qualified builder helpers such as `keaBuilders.path(...)` / `loaderBuilders.loaders(...)`.
- The inspector now walks all top-level logic sections instead of only `actions`/`reducers`/`selectors`.
- For builder sections where `typescript-go` reports the builder argument too loosely, the inspector falls back to syntax-guided object member extraction. This now gives useful results for `actions`, `reducers`, `selectors`, `listeners`, `events`, and `forms` in builder mode.
- A first Go-native semantic layer now exists in [`rewrite/internal/keainspect/model.go`](/Users/marius/Projects/Kea/kea-typegen/rewrite/internal/keainspect/model.go). It currently normalizes:
  - `path`
  - `props`
  - `key`
  - `actions`
  - object-style `true` action shorthands into literal `{ value: true }` payloads
  - `defaults`
  - `reducers`
  - `windowValues` into reducer/value/selectors state
  - `loaders` into reducer state plus synthetic request/success/failure actions
  - `lazyLoaders` into reducer state plus synthetic request/success/failure actions
  - `connect` for local imported logics by recursively inspecting the referenced source file and reusing the normalized actions/values
  - local `connect` targets reached through default imports such as `import github from './githubLogic'`
  - local `connect` / imported `logic.actionTypes.*` target expressions reached through namespace imports such as `github.githubLogic` and `github.githubLogic()`, while still reusing the connected logic's normalized imports
  - symbol-backed `connect.actions`/`connect.values` recovery from connected wrapper members, including widened connected values such as `isLoading: boolean`
  - external `connect.actions` with a listener-signature fallback when the target logic is not locally inspectable
  - external connected action payload refinement from local listener signatures when symbol-return payloads are truncated, avoiding `Record<...>` payload members in samples such as `routerConnectLogic`
  - `listeners`
  - source-guided recovery for computed `listeners` keys that point at imported `logic.actionTypes.*` references, reusing the imported logic's normalized payload/import data
  - `sharedListeners`
  - `events`
  - explicit selectors, including source-probed recovery for truncated final tuple members such as `sbla`
  - builder selectors with block-bodied final functions when `typescript-go` falls back to `any`, such as `sortedRepositories`
  - source-backed recovery when reduced generated `*LogicType` files feed back into `typescript-go` during native write rounds, preserving tuple loaders, computed imported `logic.actionTypes.*` listeners, imported selector passthroughs, reducer literal widening, object block-bodied selectors, and write-round selector/loader recoveries such as `EventIndex`, `RandomAPI`, `Partial<Record<string, S7>>`, and `loadItSuccess`'s `name: void`
  - source-guided alias preservation for `actions`, `defaults`, `reducers`, and explicit selector return annotations when `typescript-go` expands away useful ownership names
  - reduced plugin-produced fields for object `inline`/`form` and builder `typedForm`
  - builder `forms(...)` outputs are currently deferred on purpose because the TypeScript generator does not emit them in `samples/builderLogic.ts`; preserving parity matters more than speculative plugin heuristics here
- A first reduced emitter now exists in [`rewrite/internal/keainspect/emit.go`](/Users/marius/Projects/Kea/kea-typegen/rewrite/internal/keainspect/emit.go). It prints:
  - `actionCreators`
  - `actionKeys`
  - `actionTypes`
  - `actions`
  - `asyncActions`
  - `custom`
  - `defaults`
  - aggregate `reducer`
  - `listeners`
  - `key`
  - `path`
  - `pathString`
  - `props`
  - `reducers`
  - `events`
  - aggregate `selector`
  - `selectors`
  - `sharedListeners`
  - `values`
  - `_isKea`
  - `_isKeaWithKey`
  - `__keaTypeGenInternalSelectorTypes`
  - `__keaTypeGenInternalReducerActions`
  - `__keaTypeGenInternalExtraInput`
- Public reducer-state normalization now widens boolean-literal tuple reducers such as `isLoading: [false, { ...true/false handlers... }]` to `boolean` in the emitted public surface.
- The aggregate `selector` field now mirrors the TS generator's public shape more closely by exposing reducer-backed state only, while derived selectors remain under `selectors` and `values`.
- Import/reference gathering now combines normalized type-string scanning with local-module export discovery:
  - direct named imports and exported local types from the source file
  - direct default-import and namespace-import ownership reuse for qualified type references such as `Foo.Bar` and `ts.Node`
  - mixed default-plus-named import clauses now contribute both ownership reuse and fallback export discovery
  - transitive exported type names from already-imported relative modules, including namespace members and simple re-exports
  - exported type names from already-imported packages by scanning installed `.d.ts` files under `node_modules` when the current source does not spell the type import directly
  - existing imports attached by earlier normalization passes are now treated as authoritative so connected/local ownership is not re-resolved onto unrelated package exports
  - source-guided alias preservation now lets the reduced emitter keep richer ownership such as `L1`, `A2`, `D6`, `R6`, `Bla`, `ExportedApi.RandomThing`, `NodeJS.Timeout`, `S7`, and `RandomThing` in `samples/autoImportLogic.ts`, and the emitter now prints namespace/default type imports when ownership depends on them
  - native writer import-path normalization now resolves relative local modules to real files before ignore-path filtering, so external `--types` targets still respect `.kearc` `ignoreImportPaths` entries such as `./donotimport.ts`
- `samples/githubNamespaceConnectLogic.ts` now uses a checked-in local `githubNamespaceConnectLogicType.ts` like the rest of the sample corpus; stale external import-path rewrite coverage now lives in focused native write tests instead of relying on the shared sample fixture.
- Source-backed selector recovery now distrusts loose selector `returnTypeString` data when the final tuple function disagrees, and native write rounds now keep `samples/logic.ts` derived selectors such as `capitalizedName` / `upperCaseName` as `string` even after reduced generated `*LogicType` feedback.
- Reducer/action source recovery now also avoids widening mixed literal unions such as `number | 'new' | null` down to `string`, and native write rounds keep `samples/complexLogic.ts` shorthand `true` actions as `{ value: true }` instead of degrading them to `payload: any`.
- Internal selector helper recovery now uses selector dependency lists rather than only the final callback arity, strips top-level `null` / `undefined` from helper arg types to match the TS generator, gives unnamed args stable placeholders such as `arg`, deduplicates repeated dependency names as `foo2`, and keeps listener-only internal reducer actions such as `githubNamespaceConnectLogic`'s connected `setRepositories`.
- Block-bodied selector return recovery now resolves simple `const result = ...; return result` shapes, which keeps `samples/complexLogic.ts` `miscSelector` fully expanded instead of emitting invalid summarized placeholders like `... 24 more ...`.
- Plugin internal extra-input rendering now matches the TS generator more closely for object `form` plugins, including `default?: Record<string, any>` plus a typed `submit?: (form: ...) => void`.
- Reusable `typescript-go` API transport code now lives in [`rewrite/internal/tsgoapi/client.go`](/Users/marius/Projects/Kea/kea-typegen/rewrite/internal/tsgoapi/client.go).
- Preferred `tsgo` binary discovery now prefers an actually present repo binary, falls back to `tsgo`/`tsgo-upstream` on `PATH`, and reports missing-binary failures explicitly instead of silently defaulting to a dead `poc/.../tsgo-upstream` path.

## Audited coverage

- `samples/logic.ts`: `props`, `key`, `path`, `actions`, `defaults`, `reducers`, `selectors`, `loaders`, `listeners`, `sharedListeners`, `events`
- `samples/loadersLogic.ts`: `path`, `actions`, `loaders`, `reducers`
- `samples/pluginLogic.ts`: `path`, `inline`, `form`, and reduced plugin-produced `inlineAction`/`submitForm` plus `inlineReducer`/`form`
- `samples/routerConnectLogic.ts`: `path`, `connect`, `listeners`, symbol-backed external `locationChanged` action typing, `LocationChangedPayload` import recovery
- `samples/routerConnectLogic.ts`: local-listener payload refinement for expanded `Record<string, any>` fields instead of `Record<...>`
- `samples/githubConnectLogic.ts`: `path`, `connect`
- `samples/githubConnectLogic.ts`: symbol-backed connected `values` recovery for widened types such as `isLoading: boolean`
- `samples/githubNamespaceConnectLogic.ts`: namespace-imported local `connect` targets plus namespace-imported `logic.actionTypes.*` listener recovery
- namespace-imported local `connect` targets now also resolve through quoted bracket access and wrapper expressions such as `github['githubLogic']`, trailing non-null assertions, and `as` / `satisfies` type wrappers
- `samples/githubImportLogic.ts`: imported `listeners` keyed by `githubLogic.actionTypes.*`
- `samples/githubImportViaWildcardLogic.ts`: imported `listeners` keyed by `githubLogic.actionTypes.*` with wildcard-exported reducer/selector types preserved
- `samples/autoImportLogic.ts`: local `connect.actions`/`connect.values`, alias-preserving import recovery (`L1`/`A2`/`D6`/`R6`/`Bla`/`ExportedApi`/`S7`/`RandomThing`), `NodeJS.Timeout` action signature recovery, and truncated selector recovery for `sbla`
- `samples/windowValuesLogic.ts`: `path`, `windowValues`, `start`, `stop`, `saga`, `workers`
- `samples/builderLogic.ts`: builder syntax for `path`, `actions`, `reducers`, `selectors`, `listeners`, `events`, `lazyLoaders`
- `samples/builderLogic.ts`: block-bodied selector return recovery for `sortedRepositories`
- `samples/builderLogic.ts`: builder `forms(...)` outputs are intentionally omitted for now so the Go path matches the current TypeScript public surface
- `samples/complexLogic.ts`: object-style `true` action shorthand payload normalization, including typed `hideButtonActions` listener payloads
- `samples/typed-builder/typedFormDemoLogic.ts`: builder syntax for `path`, reduced `typedForm` action/state
- `samples/typed-builder/typedFormDemoLogic.ts`: reduced typed-form plugin `custom` and `__keaTypeGenInternalExtraInput` fields
- Reduced `.typegen` emission is now verified end-to-end for `samples/propsLogic.ts`.
- Basic type import collection is verified on `samples/builderLogic.ts` (`Repository` from `./types`).
- Unit coverage now also verifies default/namespace import ownership (`default as Foo`, `import type * as ts`) plus default-imported local `connect` targets.
- Reduced `.typegen` emission now also synthesizes loader actions/defaults for `samples/logic.ts`, including `Session` import collection from the source file.
- Reduced `.typegen` emission for `samples/logic.ts` now also includes `listeners`, `sharedListeners`, `events`, and the `BreakPointFunction` import from `kea`.
- Reduced `.typegen` emission now also covers:
  - `samples/windowValuesLogic.ts` reducer/value/selectors parity for `windowHeight`/`windowWidth`
  - `samples/routerConnectLogic.ts` typed connected `locationChanged` actions via connected wrapper symbol recovery plus `kea-router/lib/types` import collection
  - `samples/routerConnectLogic.ts` local-listener refinement of truncated connected payload members such as `hashParams: Record<string, any>`
  - `samples/githubNamespaceConnectLogic.ts` namespace-imported local `connect` targets with preserved `Repository` ownership plus namespace-imported listener action typing
  - `samples/loadersLogic.ts` object-style tuple loaders such as `misc`/`loadIt`, source-backed `[] -> any[]` default fallback, and preservation of explicit request-action signatures such as `addDashboard(name: string)`
  - `samples/loadersLogic.ts` tuple loader families now also survive native write rounds when `typescript-go` collapses `misc` to `Record<string, any>[]`
  - `samples/loadersLogic.ts` symbol-backed write-round return probing now recovers richer tuple-loader success payloads such as `loadItSuccess(misc: { id: number; name: void; pinned: boolean })`
  - `samples/githubLogic.ts` boolean reducer-state normalization for `isLoading` plus aggregate `selector` parity that excludes derived selectors such as `sortedRepositories`
  - `samples/githubLogic.ts` object-style block-bodied selector return recovery for `sortedRepositories` now survives native write rounds with reduced generated type feedback
  - `samples/builderLogic.ts` synthesized `lazyLoaders` actions/defaults for `lazyValue`
  - `samples/builderLogic.ts` boolean reducer-state normalization for `isLoading` plus aggregate `selector` parity that excludes derived selectors such as `sortedRepositories`
  - `samples/pluginLogic.ts` reduced object-plugin `inline`/`form` actions and reducers
  - `samples/typed-builder/typedFormDemoLogic.ts` reduced `typedForm` action, `form` reducer, `custom`, and `__keaTypeGenInternalExtraInput`
  - `samples/autoImportLogic.ts` source-guided alias preservation for `L1`/`A2`/`D6`/`R6`/`Bla`/`ExportedApi.RandomThing`/`NodeJS.Timeout`
  - `samples/autoImportLogic.ts` authoritative connected import reuse so `Repository` stays owned by `./types` instead of being spuriously re-imported from package scanning
  - `samples/autoImportLogic.ts` selector recovery for both truncated `sbla` and explicit alias return types such as `randomSpecifiedReturn`
  - `samples/autoImportLogic.ts` native write rounds now keep selector surfaces closer to the TS generator even after reduced generated type feedback, including `EventIndex`, `RandomAPI`, and `Partial<Record<string, S7>>`
  - `samples/autoImportLogic.ts` write-round selector recovery now also preserves the TS generator's inferred-vs-explicit ownership split for `randomDetectedReturn: RandomThing` versus `randomSpecifiedReturn: ExportedApi.RandomThing`
  - `samples/logic.ts` native write rounds now keep derived selector/value surfaces for `capitalizedName` / `upperCaseName` as `string` instead of collapsing to `any`
  - `samples/complexLogic.ts` now keeps `selectedActionId: number | 'new' | null` in reducer/selector/value surfaces instead of collapsing to `string`
  - `samples/complexLogic.ts` native write rounds now keep shorthand `deleteAction` / `hideButtonActions` / `incrementCounter` / `showButtonActions` payloads as `{ value: true }`
  - `samples/githubImportLogic.ts` and `samples/githubImportViaWildcardLogic.ts` imported listener action typing for computed `logic.actionTypes.*` keys
  - `samples/githubImportLogic.ts` and `samples/githubImportViaWildcardLogic.ts` imported selector passthrough plus computed imported listeners now also survive native write rounds with reduced generated type feedback
  - `samples/githubImportLogic.ts` and `samples/githubImportViaWildcardLogic.ts` omission of generic `sharedListeners` entries whose payloads only recover as `any`, keeping `BreakPointFunction` out of the emitted imports
  - identifier listener/shared-listener keys such as `updateName`, `locationChanged`, `hideButtonActions`, and `someRandomFunction` now render as identifiers instead of quoted string-literal keys
  - `samples/complexLogic.ts` literal `true` action payload typing instead of `any`
- Native CLI source-edit helpers now have focused tests for:
  - `.kearc` path/bool resolution into Go app options
  - adding missing `import type { logicType } from './logicType'` plus `kea<logicType>` annotations
  - rewriting stale generated `logicType` imports when the source still points at an old `--types` location, and deduplicating duplicate `logicType` imports left behind by earlier runs
  - inserting missing object `path` declarations for `--write-paths`
  - converting object-style inputs to builders plus adding the needed runtime imports, including reuse of existing aliases such as `path as logicPath`
  - avoiding duplicate builder `path(...)` insertion when an existing builder logic already uses an alias such as `logicPath(...)`
  - converting quoted / computed object keys such as `'path'`, `["actions"]`, and `` [`reducers`] `` to builder calls
  - reusing namespace imports for builder helpers such as `keaBuilders.path(...)`, `keaBuilders.actions(...)`, and `loaderBuilders.loaders(...)` during scanning, `--write-paths`, and object-to-builder conversion

## Latest sample sweep

- On March 11, 2026, the Go CLI successfully ran across the whole `samples/` project with:

```bash
cd /Users/marius/Projects/Kea/kea-typegen/rewrite
env GOCACHE=/tmp/kea-typegen-gocache GOMODCACHE=/tmp/kea-typegen-gomodcache go run ./cmd/kea-typegen-go check -r ../samples --verbose
```

- Result: no native CLI/runtime failures. The rewrite currently traverses the full `samples/` project and can emit type files for every currently recognized sample logic.
- Files that currently generate Go type output:
  - `samples/autoImportLogic.ts`
  - `samples/builderLogic.ts`
  - `samples/complexLogic.ts`
  - `samples/github.tsx`
  - `samples/githubConnectLogic.ts`
  - `samples/githubImportLogic.ts`
  - `samples/githubImportViaWildcardLogic.ts`
  - `samples/githubLogic.ts`
  - `samples/githubNamespaceConnectLogic.ts`
  - `samples/loadersLogic.ts`
  - `samples/logic.ts`
  - `samples/pluginLogic.ts`
  - `samples/propsLogic.ts`
  - `samples/routerConnectLogic.ts`
  - `samples/typed-builder/typedFormDemoLogic.ts`
  - `samples/windowValuesLogic.ts`
- `samples/githubNamespaceConnectLogic.ts` now emits a new `githubNamespaceConnectLogicType.ts` in the Go path. This is progress, not a crash.
- There are currently no sample files that fail by crashing the Go generator. The remaining work is parity and fidelity, not basic coverage.
- A second in-repo TS-vs-Go sweep on March 11, 2026 used sibling temp copies under the repo root plus `write -r <temp>/samples --write-paths --delete --no-import` for both implementations:
  - no `TS_ONLY` or `GO_ONLY` coverage gaps remain in the sample corpus
  - every generated file still differs textually because formatting and ordering still differ
  - semantic sample parity is now closed for the benchmarked write path, including `__keaTypeGenInternal*` helper fields

### Current benchmark status

- On March 11, 2026, after rebuilding the Go binary with `./bin/prepare-go`, the repo-local benchmark reached full semantic parity with:

```bash
./bin/benchmark -c write -n 1 -w 1
```

- Result:
  - semantic accuracy `16/16 (100.0%)`
  - exact accuracy `0/16 (0.0%)`
  - `TS_ONLY: 0`
  - `GO_ONLY: 0`
  - internal helper fields are included in that semantic comparison now
  - Go remained faster in the same run (`5.26x` on mean runtime in the last recorded local run)

### PostHog adoption baseline

- On March 11, 2026, the next real target moved from `samples/` to PostHog at `/Users/marius/Projects/PostHog/posthog`.
- Use disposable local clones for this audit. A direct JS run in the live PostHog checkout still tried to rewrite source even with `--no-import` and hit a sandbox `EPERM` on `frontend/src/lib/logic/featureFlagLogic.ts`.
- Repro setup that worked:

```bash
git clone --local --no-hardlinks /Users/marius/Projects/PostHog/posthog /tmp/posthog-typegen-js-clone
git clone --local --no-hardlinks /Users/marius/Projects/PostHog/posthog /tmp/posthog-typegen-go-clone
ln -s /Users/marius/Projects/PostHog/posthog/node_modules /tmp/posthog-typegen-js-clone/node_modules
ln -s /Users/marius/Projects/PostHog/posthog/node_modules /tmp/posthog-typegen-go-clone/node_modules
```

- JS baseline:
  - `/Users/marius/Projects/Kea/kea-typegen/bin/kea-typegen-js write -q`
  - workdir `/tmp/posthog-typegen-js-clone`
  - result: `739` generated `*Type.ts` files across `frontend/src`, `products`, and `common`
- Go baseline after the fixes in this run:
  - `env KEA_TYPEGEN_CWD=/tmp/posthog-typegen-go-clone GOCACHE=/tmp/kea-typegen-gocache GOMODCACHE=/tmp/kea-typegen-gomodcache go run ./cmd/kea-typegen-go write -q`
  - workdir `/Users/marius/Projects/Kea/kea-typegen/rewrite`
  - result: also `739` generated `*Type.ts` files, so coverage is now `TS_ONLY: 0`, `GO_ONLY: 0` on the PostHog generated-file set
- Measured semantic parity against the JS output, using the repo's `compareOutputs` logic from [`scripts/benchmark-sample-typegen.js`](/Users/marius/Projects/Kea/kea-typegen/scripts/benchmark-sample-typegen.js):
  - total files: `739`
  - semantic matches: `14`
  - semantic accuracy: `1.89%`
  - exact matches: `0`
  - semantic diffs: `725`
- The `14` semantic matches were small/simple files, for example:
  - `frontend/src/exporter/ExporterLoginType.ts`
  - `frontend/src/lib/components/Command/commandLogicType.ts`
  - `frontend/src/toolbar/shims/featureFlagLogicType.ts`
  - `frontend/src/toolbar/shims/userLogicType.ts`
- Runtime note:
  - the `go run ./cmd/kea-typegen-go ...` development path was misleading here because it includes compile overhead; that path exceeded the earlier `180s` timeout on a fresh clone
  - the compiled binary is better, but fresh-project runtime is still not healthy enough:
    - `env KEA_TYPEGEN_CWD=/tmp/posthog-typegen-go-clone3 /Users/marius/Projects/Kea/kea-typegen/rewrite/bin/kea-typegen-go write -q` produced all `739` generated files on a fresh clone, but still did not exit within `180s`
    - reruns are much better: `./bin/kea-typegen-go write -q` and `./bin/kea-typegen-go check -q` completed quickly on an already-generated clone
  - treat first full-project write performance / shutdown behavior as a real follow-up item
- Source-scanner/parser fixes that were required just to get PostHog to generate:
  - regex literal skipping in raw source scans, fixing `frontend/src/scenes/session-recordings/player/inspector/playerInspectorLogic.ts`
  - JSX slash false-positive avoidance, fixing `frontend/src/layout/navigation-3000/navigationLogic.tsx`
  - JSX apostrophe false-positive avoidance, fixing `frontend/src/scenes/teamLogic.tsx`
  - better block-comment / unterminated-token diagnostics, including local previews in error messages
  - typed assignment logic-name detection, so `const foo: LogicWrapper<T> = kea(...)` no longer collapses to generic `logicType`
- High-priority PostHog parity gaps found in this sweep:
  - builder `props` recovery is still far too weak:
    - Go emitted `props: any` in `391` generated files
    - the JS output emitted `props: any` in `0` generated files in the same clone comparison
  - builder `key` recovery is also weak:
    - Go emitted `key: any` in `232` generated files
  - import-path normalization still diverges heavily:
    - Go emitted aliased imports like `~/`, `lib/`, `scenes/`, or `products/` in `139` generated files
    - JS emitted `0` such aliased generated imports in the same comparison
  - internal helper emission is badly over-broad:
    - Go emitted `__keaTypeGenInternal*` fields in `395` generated files
    - JS emitted them in only `19`
  - `lazyLoaders` needs source-backed recovery similar to object `loaders`:
    - representative failure: [`frontend/src/scenes/billing/billingLogic.tsx`](/tmp/posthog-typegen-js-clone/frontend/src/scenes/billing/billingLogic.tsx)
    - `typescript-go` reports the `billing` lazy-loader member as `any[]`, so the Go path misses actions such as `loadBilling`, `updateBillingLimits`, `deactivateProduct`, and `switchFlatrateSubscriptionPlan`
  - `kea-forms` parity is still a major blocker outside the sample corpus:
    - representative failure: [`frontend/src/lib/components/QuickFilters/quickFilterFormLogic.ts`](/tmp/posthog-typegen-js-clone/frontend/src/lib/components/QuickFilters/quickFilterFormLogic.ts)
    - Go misses form-driven surface area and connected actions such as `loadPropertyValues`
  - connected logic recovery still needs to work through PostHog path aliases and broader local-module patterns:
    - representative misses include `featureFlags`, `preflight`, `currentOrganization`, and `propertyDefinitionsModel.loadPropertyValues`
- Follow-up progress on March 11, 2026:
  - direct/member and template-string `key(...)` recovery is better in real PostHog files now:
    - [`frontend/src/lib/components/QuickFilters/quickFilterFormLogic.ts`](/Users/marius/Projects/PostHog/posthog/frontend/src/lib/components/QuickFilters/quickFilterFormLogic.ts): `keyType` now resolves to `string`
    - [`frontend/src/lib/monaco/codeEditorLogic.tsx`](/Users/marius/Projects/PostHog/posthog/frontend/src/lib/monaco/codeEditorLogic.tsx): `keyType` now resolves to `string`
  - the fixes behind that were not just `key(...)`-specific:
    - multiline local value initializers no longer truncate at the first top-level newline, which matters for curried helper exports
    - multiline interface expansion no longer flattens callback-bearing members into malformed inline object text, which was poisoning member lookups like `props.key`
    - there is now a state-aware source fallback for imported callback helpers that can resolve named imports through tsconfig `baseUrl` / `paths`
  - there is a new synthetic regression test for the imported-helper path:
    - aliased `key(keyForInsightLogicProps('new'))` source recovery now passes in-package with a temp tsconfig-style `paths` mapping
  - still unresolved in the real PostHog legacy-model path:
    - [`frontend/src/scenes/insights/insightLogic.tsx`](/Users/marius/Projects/PostHog/posthog/frontend/src/scenes/insights/insightLogic.tsx): `keyType` is still `any`
    - [`frontend/src/lib/components/AddToDashboard/addToDashboardModalLogic.ts`](/Users/marius/Projects/PostHog/posthog/frontend/src/lib/components/AddToDashboard/addToDashboardModalLogic.ts): `keyType` is still `any`
  - that discrepancy now looks narrower than before:
    - the raw source fallback path works against a synthetic aliased-helper test
    - the remaining failure is likely in the live `BuildParsedLogics(report)` runtime context for actual PostHog files, not in the basic source inference logic itself
  - next debugging target for this exact blocker:
    - instrument the actual `BuildParsedLogics(report)` path for the two files above and confirm whether runtime config/path metadata differs from the synthetic test setup, or whether some earlier step still prevents the helper-based fallback from being used
  - fresh PostHog runtime behavior improved materially after native write-path fast paths:
    - files with no detected Kea logic now return before tsgo client startup / per-file project lookup
    - files that do not even contain the token `kea` now skip the heavier logic scan entirely
    - when the tsgo snapshot only exposes one project, per-file `getDefaultProjectForFile` calls are skipped
  - fresh clean full-run status on `/tmp/posthog-go-fresh2.yk8yw3`:
    - `739` generated `*Type.ts` files appeared by about `00:54` on a truly empty clone
    - the run still did not converge cleanly after that; by about `04:14` it had logged `845` writes for `739` files
    - the remaining issue is not startup anymore, it is late-round output churn
  - the clearest current convergence blocker is a single-file repro:
    - command:

```bash
cd /Users/marius/Projects/Kea/kea-typegen/rewrite
env KEA_TYPEGEN_CWD=/tmp/posthog-go-fresh2.yk8yw3 ./bin/kea-typegen-go write -q -f /tmp/posthog-go-fresh2.yk8yw3/frontend/src/scenes/experiments/ExperimentForm/variantsPanelLogic.ts
```

  - repeated single-file writes mutate [`frontend/src/scenes/experiments/ExperimentForm/variantsPanelLogicType.ts`](/tmp/posthog-go-fresh2.yk8yw3/frontend/src/scenes/experiments/ExperimentForm/variantsPanelLogicType.ts) across runs instead of converging
  - observed unstable surfaces in that file:
    - `validateFeatureFlagKeySuccess` toggles between a widened `valid: boolean` union and a more expanded union with summarized `{ ...; }` members
    - `unavailableFeatureFlagKeys` toggles between `SetConstructor` and `Set<any>`
    - `__keaTypeGenInternalSelectorTypes.unavailableFeatureFlagKeys` appears/disappears across reruns
  - current working theory for the oscillation:
    - loader success-action payload typing is still overly dependent on tsgo-fed member text instead of a stable source-backed async return union
    - selector return / internal-helper recovery around `new Set([...])` is still unstable when reduced generated `*Type.ts` feedback is present

### Main parity gaps found in the sample sweep

- The sample benchmark no longer has any semantic parity gaps on the covered write path.
- Remaining rewrite work is now outside the `16/16` sample benchmark bar:
  - broader syntax/ownership coverage beyond the currently covered sample shapes
  - tighter exact-source parity if it ever becomes desirable, even though the current target is semantic rather than byte-level equality
  - broader plugin and builder generalization beyond the specific sample-backed cases the benchmark exercises

## Immediate next steps

1. Keep `./bin/benchmark -c write -n 1 -w 1` at `16/16` semantic matches as a regression gate for the rewrite.
2. Make PostHog the next parity target:
   - first focus on builder `props` / `key` / `connect` / `lazyLoaders` / `kea-forms`, because those explain most of the current `14/739` semantic baseline
   - keep using disposable `/tmp/posthog-typegen-*` clones plus a semantic compare against JS output; do not iterate directly in the live PostHog checkout
   - add a reusable PostHog semantic-compare helper or script instead of reusing an ad hoc Node heredoc
   - stabilize fresh-run convergence before chasing broader parity again:
     - fix the `variantsPanelLogic.ts` rewrite loop first
     - then rerun a truly clean full-project `write` and record total runtime plus total write count
   - start a repo-local plugin normalization registry:
     - keep it in this repo for now; do not solve dynamic loading yet
     - move plugin-specific behavior such as `kea-forms`, loader/lazy-loader extras, and future PostHog plugin surfaces behind explicit plugin handlers instead of scattering more one-off heuristics
3. Expand coverage beyond the current sample benchmark:
   - richer connected target expressions and ownership fidelity
   - broader selector/block-body recovery outside the currently covered patterns
   - additional plugin/builder cases not represented in `samples/`
4. Decide whether exact-source parity is worth pursuing:
   - formatting/order still differ even though AST semantics now match on the benchmarked path
   - if exact parity matters later, move native source edits and emission closer to AST-backed JS behavior

## Known gaps

- Code generation still exists only for a reduced subset of the final public interface, although the emitter now also covers `actionKeys` plus aggregate `reducer`/`selector` fields.
- The current target is public AST parity, not byte-for-byte source parity.
- `__keaTypeGenInternal*` fields are part of the current sample-benchmark parity target and are now included in the semantic comparison.
- Builder syntax is detected and partially introspected, and `typedForm(...)` now has a reduced heuristic path, but custom builders are still mostly opaque without plugin-specific handling.
- PostHog showed that builder `props(...)` and `key(...)` still fall back to `any` far too often in real code, even when the source spells useful types locally.
- Import gathering is still not fully symbol-backed. Local relative module export scanning, installed-package `.d.ts` export scanning, and source-guided named/default/namespace alias recovery now cover much more of the local case, but inferred ownership and some package/global/plugin helper types are still missing.
- Generated import-path normalization still does not mirror the JS generator in larger repos. PostHog currently emits many aliased generated imports (`~/`, `lib/`, `scenes/`, `products/`) where JS rewrites them to relative paths.
- Existing normalized imports now prevent some bad fallback resolutions, but ownership still depends on heuristics once a type has not already been attached by an earlier pass.
- Connect normalization now uses symbol-backed wrapper members for local imports, default-imported local targets, namespace-imported local targets, and simple package-provided logics, but more complex target expressions and fuller ownership/import fidelity still need work.
- PostHog also showed that aliased local-module `connect` targets and connected values/actions through app-specific path aliases still miss a lot of surface area.
- Alias-preserving ownership is much better in richer files such as `samples/autoImportLogic.ts`, but still depends on source spelling; inferred-only aliases can still collapse to expanded forms.
- Loader parity is still incomplete in some richer cases, especially around keeping request-action signatures as flat as the TS generator when loaders overlap explicit `actions`.
- `lazyLoaders` is materially weaker than object `loaders` in real repos because the Go path still lacks the same kind of source-backed fallback when `typescript-go` reports members as `any[]`.
- Source-backed recovery now keeps several native write rounds semantically stable even after reduced generated `*LogicType` files feed back into `typescript-go`, including the previously-regressed `samples/loadersLogic.ts` `loadItSuccess` payload and `samples/autoImportLogic.ts` selector surfaces.
- Reducer/default/value normalization still leaks reducer-spec internals into emitted public types in some samples.
- Plugin-produced fields now have reduced heuristics for a few known samples, including object `inline`/`form` plus typed-form `custom`/`__keaTypeGenInternalExtraInput`, but there is still no general plugin execution or typegen hook model.
- The next structural step should be a repo-local plugin system:
  - keep plugin handlers statically linked in-repo for now
  - defer external loading/discovery until the handler boundaries are stable
  - use that registry to own `kea-forms`, loader/lazy-loader variants, and other plugin-produced public/internal fields explicitly instead of expanding ad hoc conditionals in `model.go`
- Builder `forms(...)` output is intentionally deferred again because the current TypeScript generator does not expose those public fields in `samples/builderLogic.ts`; broader `kea-forms` parity remains future work once plugin behavior is handled intentionally.
- That deferral is no longer a theoretical gap only. PostHog uses `kea-forms` broadly enough that public parity will stay very low until builder `forms(...)` behavior is handled intentionally for non-sample code too.
- Explicit selector normalization is better for expression-bodied tuples, explicit return annotations, and simple block-bodied returns, but deeper alias-sensitive cases still need more work.
- `check` / `write` / polling `watch` now run natively, and object-style `--convert-to-builders` now has a source-text rewrite path, but source edits are still string-based rather than AST-backed, so formatting and edge-case syntax coverage are not yet fully JS-identical.
- Native `--convert-to-builders` now handles identifier and quoted/computed string top-level logic keys plus aliased and namespace-qualified builder helpers, but broader syntax shapes still need AST-backed parity.
- PostHog fresh-run startup is now better, but end-to-end convergence is still unstable:
  - a truly empty clean clone now reaches the full `739` generated-file set in under a minute
  - however, late rounds still rewrite a small tail of files and keep the native writer alive for multiple extra tsgo passes
  - `frontend/src/scenes/experiments/ExperimentForm/variantsPanelLogicType.ts` is a confirmed single-file repro for that oscillation
- `--show-ts-errors` does not yet reproduce the JS compiler-watch diagnostics output.
- The scanner still uses byte offsets from UTF-8 source text and does not yet cover arbitrary computed/non-string property keys or deeper member-expression / optional-chaining builder calls beyond simple namespace-qualified helpers.

## How to continue next run

- Start with [`GO_REWRITE_TODO.md`](/Users/marius/Projects/Kea/kea-typegen/GO_REWRITE_TODO.md).
- Then inspect:
  - [`rewrite/internal/keainspect/model.go`](/Users/marius/Projects/Kea/kea-typegen/rewrite/internal/keainspect/model.go)
  - [`rewrite/internal/keainspect/emit.go`](/Users/marius/Projects/Kea/kea-typegen/rewrite/internal/keainspect/emit.go)
  - [`rewrite/internal/keainspect/inspect.go`](/Users/marius/Projects/Kea/kea-typegen/rewrite/internal/keainspect/inspect.go)
  - [`rewrite/internal/keainspect/source.go`](/Users/marius/Projects/Kea/kea-typegen/rewrite/internal/keainspect/source.go)
  - [`rewrite/internal/keainspect/model_test.go`](/Users/marius/Projects/Kea/kea-typegen/rewrite/internal/keainspect/model_test.go)
- Useful commands:

```bash
cd /Users/marius/Projects/Kea/kea-typegen
./bin/prepare-js
./bin/prepare-go
./bin/kea-typegen-js check -r ./samples
./bin/kea-typegen-go check -r ./samples --verbose
./bin/benchmark -n 5 -w 1
git clone --local --no-hardlinks /Users/marius/Projects/PostHog/posthog /tmp/posthog-typegen-js-clone
git clone --local --no-hardlinks /Users/marius/Projects/PostHog/posthog /tmp/posthog-typegen-go-clone
ln -s /Users/marius/Projects/PostHog/posthog/node_modules /tmp/posthog-typegen-js-clone/node_modules
ln -s /Users/marius/Projects/PostHog/posthog/node_modules /tmp/posthog-typegen-go-clone/node_modules
cd /tmp/posthog-typegen-js-clone
/Users/marius/Projects/Kea/kea-typegen/bin/kea-typegen-js write -q
cd /Users/marius/Projects/Kea/kea-typegen/rewrite
env GOCACHE=/tmp/kea-typegen-gocache GOMODCACHE=/tmp/kea-typegen-gomodcache go test ./...
env KEA_TYPEGEN_CWD=/tmp/posthog-typegen-go-clone GOCACHE=/tmp/kea-typegen-gocache GOMODCACHE=/tmp/kea-typegen-gomodcache go run ./cmd/kea-typegen-go check -q
env KEA_TYPEGEN_CWD=/tmp/posthog-typegen-go-clone GOCACHE=/tmp/kea-typegen-gocache GOMODCACHE=/tmp/kea-typegen-gomodcache go run ./cmd/kea-typegen-go write -q
../bin/kea-typegen-go -file ../samples/propsLogic.ts -format typegen
../bin/kea-typegen-go -file ../samples/builderLogic.ts -format model
```
