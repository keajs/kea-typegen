# Go Rewrite TODO

## Target

- The working goal is now: both the Go rewrite and the existing TypeScript implementation should be able to generate types for every logic-bearing file under `samples/`.
- Byte-for-byte output equality is not required. The target is semantic parity, with emitted TypeScript ASTs staying as close as practical between the Go and TS versions.
- `__keaTypeGenInternal*` helper fields are not a parity requirement going forward. If the Go emitter can avoid them while still matching the user-visible/public type surface, that is acceptable.
- Plugin parity is explicitly a last-step concern. First get all non-plugin samples generating the right public type shapes; only then spend time on generalized plugin/typegen-hook parity.

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
  - every generated file still differs textually because formatting, ordering, and omitted `__keaTypeGenInternal*` helpers are still different
  - the newly fixed `samples/logic.ts` write-round selector drift is no longer a meaningful public-surface gap; its remaining diff is now formatting/internal-helper noise
  - the highest-signal remaining public gap from the current sweep is `samples/complexLogic.ts`, where reducer/action nullability and `DashboardItemType` payload shaping still diverge semantically

### Main parity gaps found in the sample sweep

- `connect` and imported-listener fidelity still need work:
  - broader target-expression support and ownership fidelity still need work beyond the current identifier / call / namespace-property / quoted-bracket / default-import / simple assertion-wrapper coverage
  - the remaining gap is no longer basic payload recovery for `routerConnectLogic.ts` or the import-listener samples; it is richer ownership and declaration-source fidelity in more complex connected cases
- Reducer/default/value normalization is much better across the covered samples, but broader sample-sweep verification is still needed for the remaining object-style and connected edge cases.
- `samples/complexLogic.ts` still has real public-surface drift in the current sweep:
  - `inspectForElementWithIndex`, `newAction`, and related action payload nullability still differ from the TS generator
  - `updateDashboardInsight` still expands away the intended `DashboardItemType` payload owner in the emitted action surface
- Loader parity is still incomplete:
  - Some loader action signatures still prefer destructured object payloads where the TS generator currently emits a flatter public surface.
- Import ownership and selector return fidelity are still heuristic in richer files:
  - `samples/autoImportLogic.ts` is much closer now in native write rounds, but broader ownership/import fidelity still depends on source spelling and heuristics in richer files.
  - Union ordering / explicit-vs-inferred naming still differs in a few places, which is acceptable only if the emitted AST remains semantically close.
- The emitter is still reduced versus the current TS output:
  - missing `__keaTypeGenInternal*` fields are acceptable by the new goal
  - missing or widened public fields are not acceptable
  - the remaining work should focus on public interface parity, not reproducing every TS-only internal helper node
- Plugin outputs still differ, but they should stay lower priority until the non-plugin sample set is in good shape:
  - `samples/builderLogic.ts` currently omits `forms(...)` plugin-produced public fields on purpose, matching the current TypeScript generator instead of overproducing speculative `kea-forms` API
  - `samples/pluginLogic.ts`
  - `samples/typed-builder/typedFormDemoLogic.ts`

## Immediate next steps

1. Get non-plugin sample parity to the point where both generators can emit public types for every sample file with ASTs that are semantically very close:
   - do not chase byte-level formatting equality
   - do not treat `__keaTypeGenInternal*` fields as required output
   - use `samples/` as the acceptance suite
2. Fix reducer/default/value normalization regressions that still leak reducer-spec internals or widened `any`/`never[]` into emitted public types:
   - keep checking the remaining non-plugin sample corpus for object-style and connected edge cases that still leak reducer-spec artifacts
   - prioritize cases where the public surface still widens to `any`, keeps literal defaults where TS emits a broader state type, or lets derived selectors leak into aggregate reducer-backed fields
3. Tighten `connect` and imported listener parity:
   - broader target-expression support beyond the current identifier / call / namespace-property / quoted-bracket / default-import / simple assertion-wrapper cases
   - connected values/actions should preserve declaration-source ownership/import fidelity rather than reducer-spec artifacts
   - keep checking the remaining connected cases for ownership-preserving listener/value surfaces, especially when the right public owner is only available semantically
   - keep broadening target-expression support without regressing the now-covered router and import-listener sample cases
4. Replace the remaining source-scanned import gathering with symbol-backed ownership data from `typescript-go`, especially when the right owner is only available semantically rather than from explicit source text:
   - inferred aliases that are not spelled in the source
   - package/global helper types beyond installed-package export scanning
   - ownership-preserving reuse inside connected external logics beyond the current authoritative-import reuse and the new explicit default/namespace-owner reuse
5. Finish loader and selector parity for the remaining richer non-plugin samples:
   - keep tightening loader request-action signatures where the Go path still destructures or widens more than the TS path
   - keep improving the remaining selector return recovery and ownership/import fidelity in richer files after the new `samples/autoImportLogic.ts` write-round fixes
   - tighten the remaining non-loader public action signatures where the Go path still destructures or widens more than the TS path
6. Tighten the new native CLI parity layer:
   - replace string-based source edits with AST-backed rewrites so import/path repairs match JS formatting and broader syntax shapes
   - support the remaining JS write-mode gaps beyond the new native `--convert-to-builders` path, especially richer `--show-ts-errors` reporting
   - broaden watch invalidation beyond the current polling loop and root-tree scanning
7. Leave plugin and typed-builder generalization for last:
   - richer `kea-forms` selectors/action payload details and literal-preserving defaults
   - builder plugins whose semantics are not directly inspectable from the callsite
   - any generalized plugin execution or typegen-hook model

## Known gaps

- Code generation still exists only for a reduced subset of the final public interface, although the emitter now also covers `actionKeys` plus aggregate `reducer`/`selector` fields.
- The current target is public AST parity, not byte-for-byte source parity.
- `__keaTypeGenInternal*` fields are no longer considered required parity output.
- Builder syntax is detected and partially introspected, and `typedForm(...)` now has a reduced heuristic path, but custom builders are still mostly opaque without plugin-specific handling.
- Import gathering is still not fully symbol-backed. Local relative module export scanning, installed-package `.d.ts` export scanning, and source-guided named/default/namespace alias recovery now cover much more of the local case, but inferred ownership and some package/global/plugin helper types are still missing.
- Existing normalized imports now prevent some bad fallback resolutions, but ownership still depends on heuristics once a type has not already been attached by an earlier pass.
- Connect normalization now uses symbol-backed wrapper members for local imports, default-imported local targets, namespace-imported local targets, and simple package-provided logics, but more complex target expressions and fuller ownership/import fidelity still need work.
- Alias-preserving ownership is much better in richer files such as `samples/autoImportLogic.ts`, but still depends on source spelling; inferred-only aliases can still collapse to expanded forms.
- Loader parity is still incomplete in some richer cases, especially around keeping request-action signatures as flat as the TS generator when loaders overlap explicit `actions`.
- Source-backed recovery now keeps several native write rounds semantically stable even after reduced generated `*LogicType` files feed back into `typescript-go`, including the previously-regressed `samples/loadersLogic.ts` `loadItSuccess` payload and `samples/autoImportLogic.ts` selector surfaces.
- Reducer/default/value normalization still leaks reducer-spec internals into emitted public types in some samples.
- Plugin-produced fields now have reduced heuristics for a few known samples, including object `inline`/`form` plus typed-form `custom`/`__keaTypeGenInternalExtraInput`, but there is still no general plugin execution or typegen hook model. This is intentionally lower priority than non-plugin sample parity.
- Builder `forms(...)` output is intentionally deferred again because the current TypeScript generator does not expose those public fields in `samples/builderLogic.ts`; broader `kea-forms` parity remains future work once plugin behavior is handled intentionally.
- Explicit selector normalization is better for expression-bodied tuples, explicit return annotations, and simple block-bodied returns, but deeper alias-sensitive cases still need more work.
- `check` / `write` / polling `watch` now run natively, and object-style `--convert-to-builders` now has a source-text rewrite path, but source edits are still string-based rather than AST-backed, so formatting and edge-case syntax coverage are not yet fully JS-identical.
- Native `--convert-to-builders` now handles identifier and quoted/computed string top-level logic keys plus aliased and namespace-qualified builder helpers, but broader syntax shapes still need AST-backed parity.
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
cd /Users/marius/Projects/Kea/kea-typegen/rewrite
env GOCACHE=/tmp/kea-typegen-gocache GOMODCACHE=/tmp/kea-typegen-gomodcache go test ./...
../bin/kea-typegen-go -file ../samples/propsLogic.ts -format typegen
../bin/kea-typegen-go -file ../samples/builderLogic.ts -format model
```
