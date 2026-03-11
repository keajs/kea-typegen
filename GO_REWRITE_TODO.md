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
  - source-guided alias preservation for `actions`, `defaults`, `reducers`, and explicit selector return annotations when `typescript-go` expands away useful ownership names
  - reduced plugin-produced fields for object `inline`/`form`, builder `forms`, and builder `typedForm`
  - source-backed builder `forms` default-shape recovery for spread-heavy defaults, keeping full field coverage such as `key39` and literal defaults such as `asd: true`
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
- Import/reference gathering now combines normalized type-string scanning with local-module export discovery:
  - direct named imports and exported local types from the source file
  - direct default-import and namespace-import ownership reuse for qualified type references such as `Foo.Bar` and `ts.Node`
  - mixed default-plus-named import clauses now contribute both ownership reuse and fallback export discovery
  - transitive exported type names from already-imported relative modules, including namespace members and simple re-exports
  - exported type names from already-imported packages by scanning installed `.d.ts` files under `node_modules` when the current source does not spell the type import directly
  - existing imports attached by earlier normalization passes are now treated as authoritative so connected/local ownership is not re-resolved onto unrelated package exports
  - source-guided alias preservation now lets the reduced emitter keep richer ownership such as `L1`, `A2`, `D6`, `R6`, `Bla`, `ExportedApi.RandomThing`, `NodeJS.Timeout`, `S7`, and `RandomThing` in `samples/autoImportLogic.ts`, and the emitter now prints namespace/default type imports when ownership depends on them
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
- `samples/githubImportLogic.ts`: imported `listeners` keyed by `githubLogic.actionTypes.*`
- `samples/githubImportViaWildcardLogic.ts`: imported `listeners` keyed by `githubLogic.actionTypes.*` with wildcard-exported reducer/selector types preserved
- `samples/autoImportLogic.ts`: local `connect.actions`/`connect.values`, alias-preserving import recovery (`L1`/`A2`/`D6`/`R6`/`Bla`/`ExportedApi`/`S7`/`RandomThing`), `NodeJS.Timeout` action signature recovery, and truncated selector recovery for `sbla`
- `samples/windowValuesLogic.ts`: `path`, `windowValues`, `start`, `stop`, `saga`, `workers`
- `samples/builderLogic.ts`: builder syntax for `path`, `actions`, `reducers`, `selectors`, `listeners`, `events`, `lazyLoaders`, reduced `forms` actions/state/selectors/imports
- `samples/builderLogic.ts`: block-bodied selector return recovery for `sortedRepositories`
- `samples/builderLogic.ts`: source-backed `forms` default expansion through spread items such as `item`, preserving the full `DashboardItemType` shape and literal `asd: true`
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
  - `samples/builderLogic.ts` synthesized `lazyLoaders` actions/defaults for `lazyValue`
  - `samples/pluginLogic.ts` reduced object-plugin `inline`/`form` actions and reducers
  - `samples/builderLogic.ts` reduced `forms` actions, reducer state, derived selectors, and `kea-forms` imports
  - `samples/builderLogic.ts` source-backed full `forms` default shapes for actions/state, avoiding `... 25 more ...` truncation and preserving literal defaults
  - `samples/typed-builder/typedFormDemoLogic.ts` reduced `typedForm` action, `form` reducer, `custom`, and `__keaTypeGenInternalExtraInput`
  - `samples/autoImportLogic.ts` source-guided alias preservation for `L1`/`A2`/`D6`/`R6`/`Bla`/`ExportedApi.RandomThing`/`NodeJS.Timeout`
  - `samples/autoImportLogic.ts` authoritative connected import reuse so `Repository` stays owned by `./types` instead of being spuriously re-imported from package scanning
  - `samples/autoImportLogic.ts` selector recovery for both truncated `sbla` and explicit alias return types such as `randomSpecifiedReturn`
  - `samples/githubImportLogic.ts` and `samples/githubImportViaWildcardLogic.ts` imported listener action typing for computed `logic.actionTypes.*` keys
  - `samples/complexLogic.ts` literal `true` action payload typing instead of `any`
- Native CLI source-edit helpers now have focused tests for:
  - `.kearc` path/bool resolution into Go app options
  - adding missing `import type { logicType } from './logicType'` plus `kea<logicType>` annotations
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

### Main parity gaps found in the sample sweep

- `connect` and imported-listener fidelity still need work:
  - `samples/routerConnectLogic.ts` still degrades `LocationChangedPayload` back to `{ [x: string]: any }` in the emitted public action/listener types.
  - `samples/githubImportLogic.ts` and `samples/githubImportViaWildcardLogic.ts` still lose imported listener payload typing and fall back to `any` for some selector/shared-listener surfaces.
  - `samples/githubConnectLogic.ts` and `samples/githubNamespaceConnectLogic.ts` still mis-model connected boolean values such as `isLoading` / `githubIsLoading` as reducer-spec unions instead of plain booleans.
- Reducer/default/key inference still regresses in object-style samples:
  - `samples/logic.ts` still leaks reducer-spec structures into emitted `defaults` / `reducers` / `values`, and `key` still widens to `any`.
  - `samples/propsLogic.ts` still infers `currentPage` as `any[]` and widens `key` to `any`.
  - `samples/windowValuesLogic.ts` still widens `windowHeight` to `any`.
  - `samples/githubLogic.ts` and `samples/builderLogic.ts` still have some reducer/value/selectors emitted from reducer-spec internals rather than normalized state values.
- Loader parity is still incomplete:
  - `samples/loadersLogic.ts` still drops the `loadIt` loader family and still infers `shouldNotBeNeverButAny` as `never[]`.
  - Some loader action signatures still prefer destructured object payloads where the TS generator currently emits a flatter public surface.
- Import ownership and selector return fidelity are still heuristic in richer files:
  - `samples/autoImportLogic.ts` still loses some ownership/alias fidelity on selector returns and imports, including cases that should stay closer to `RandomAPI`, `S7`, and `EventIndex`.
  - Union ordering / explicit-vs-inferred naming still differs in a few places, which is acceptable only if the emitted AST remains semantically close.
- The emitter is still reduced versus the current TS output:
  - missing `__keaTypeGenInternal*` fields are acceptable by the new goal
  - missing or widened public fields are not acceptable
  - the remaining work should focus on public interface parity, not reproducing every TS-only internal helper node
- Plugin outputs still differ, but they should stay lower priority until the non-plugin sample set is in good shape:
  - `samples/pluginLogic.ts`
  - `samples/typed-builder/typedFormDemoLogic.ts`

## Immediate next steps

1. Get non-plugin sample parity to the point where both generators can emit public types for every sample file with ASTs that are semantically very close:
   - do not chase byte-level formatting equality
   - do not treat `__keaTypeGenInternal*` fields as required output
   - use `samples/` as the acceptance suite
2. Fix reducer/default/value normalization regressions that still leak reducer-spec internals or widened `any`/`never[]` into emitted public types:
   - `samples/logic.ts`
   - `samples/propsLogic.ts`
   - `samples/windowValuesLogic.ts`
   - `samples/githubLogic.ts`
   - `samples/builderLogic.ts`
3. Tighten `connect` and imported listener parity:
   - broader target-expression support beyond the current identifier / call / namespace-property / default-import cases
   - connected values/actions should preserve declaration-source ownership/import fidelity rather than reducer-spec artifacts
   - `samples/routerConnectLogic.ts` should keep `LocationChangedPayload`-level public typing instead of falling back to `{ [x: string]: any }`
   - `samples/githubImportLogic.ts`, `samples/githubImportViaWildcardLogic.ts`, `samples/githubConnectLogic.ts`, and `samples/githubNamespaceConnectLogic.ts` should regain the public listener/value shapes currently emitted by the TS generator
4. Replace the remaining source-scanned import gathering with symbol-backed ownership data from `typescript-go`, especially when the right owner is only available semantically rather than from explicit source text:
   - inferred aliases that are not spelled in the source
   - package/global helper types beyond installed-package export scanning
   - ownership-preserving reuse inside connected external logics beyond the current authoritative-import reuse and the new explicit default/namespace-owner reuse
5. Finish loader and selector parity for the remaining richer non-plugin samples:
   - restore the missing `loadIt` loader family and fix `never[]` fallback in `samples/loadersLogic.ts`
   - keep improving selector return recovery and alias preservation in `samples/autoImportLogic.ts`
   - tighten public action signatures where the Go path currently destructures or widens more than the TS path
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
- Loader parity is still incomplete in richer cases such as `samples/loadersLogic.ts`, including missing synthetic action families and some `never[]` fallbacks.
- Reducer/default/value normalization still leaks reducer-spec internals into emitted public types in some samples.
- Plugin-produced fields now have reduced heuristics for a few known samples, including typed-form `custom`/`__keaTypeGenInternalExtraInput`, but there is still no general plugin execution or typegen hook model. This is intentionally lower priority than non-plugin sample parity.
- Builder `forms` defaults are now much closer for local spread-heavy defaults, but imported spreads and broader expression shapes still fall back to heuristic type text.
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
cd /Users/marius/Projects/Kea/kea-typegen/rewrite
env GOCACHE=/tmp/kea-typegen-gocache GOMODCACHE=/tmp/kea-typegen-gomodcache go test ./...
env GOCACHE=/tmp/kea-typegen-gocache GOMODCACHE=/tmp/kea-typegen-gomodcache go run ./cmd/kea-typegen-go -file ../samples/propsLogic.ts -format typegen
env GOCACHE=/tmp/kea-typegen-gocache GOMODCACHE=/tmp/kea-typegen-gomodcache go run ./cmd/kea-typegen-go -file ../samples/builderLogic.ts -format model
```
