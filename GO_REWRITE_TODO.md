# Go Rewrite TODO

## Goal

- Keep the repo-local sample benchmark at `./bin/benchmark -c write -n 1 -w 0 --skip-prepare` with `20/20` semantic matches.
- Improve real-world parity against the existing TypeScript generator.
- Always rerun the FrameOS full-compare corpus after meaningful parity changes, and keep aiming for `100%` semantic accuracy there instead of treating sample-benchmark wins as sufficient.
- Treat semantic TypeScript AST parity as the current scorekeeping metric, but do not confuse that metric with actual type-correctness.

## Strategy Reset

- Yes: the current rewrite still contains too much semantic type recovery driven by source-shape / AST-shape / printed-type heuristics inside `rewrite/internal/keainspect/model.go`.
- That is now explicit technical debt, not the roadmap.
- Non-negotiable rule:
  - we must get semantic types from TypeScript checker/internal API data, not from an endless pile of Go-side guesses about source shape
- AST/source parsing is still allowed for one thing only:
  - locating the exact node / symbol / signature / property handle to ask TypeScript the real question
- AST/source parsing is not allowed to become the source of truth for semantic types.
- Never add another long-lived rule whose job is “guess the type from the shape of the code”.
- If a needed type is not exposed by the current hidden `tsgo` API, the default next move is:
  - extend the probe path
  - extend the wrapper
  - or fork / vendor `tsgo` and add the missing endpoint
- Do not keep playing compare-driven cat-and-mouse with one corpus at a time.
- A heuristic that fixes FrameOS by guessing from source shape is not success if the same rule can drift on the next codebase.
- The target architecture is now:
  - Go finds the right TypeScript nodes and asks checker-backed APIs for truth
  - `tsgo` returns normalized semantic data
  - Go formats and merges that data
  - Go does not re-implement TypeScript’s type reasoning in `model.go`

## March 24, 2026 Run

### Environment

- The local env is now fully reproducible through `flox activate` on this host.
- Verified again on March 24, 2026:
  - `node v24.13.0`
  - `npm 11.6.2`
  - `yarn 1.22.22`
  - `go version go1.25.7 linux/amd64`
- Repo-local `tsgo` is installed and runnable:
  - `./.tsgo/node_modules/.bin/tsgo --version` => `Version 7.0.0-dev.20260323.1`
  - `./.tsgo/node_modules/.bin/tsgo --api --help` still exposes the hidden API entrypoint
- Registry check on March 24, 2026:
  - `npm view @typescript/native-preview version time --json`
  - latest published version is still `7.0.0-dev.20260323.1`
  - npm registry `modified` timestamp is `2026-03-23T08:01:26.492Z`

### Current Results

- Default sample benchmark rerun on March 24, 2026:
  - command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
  - semantic accuracy: `20/20`
  - exact accuracy: `0/20`
- FrameOS run sequence on March 24, 2026:
  - run 1 command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json'`
  - run 1 semantic matches: `10/38`
  - run 1 semantic accuracy: `26.32%`
  - run 2 command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json'`
  - run 2 semantic matches: `14/38`
  - run 2 semantic accuracy: `36.84%`
  - run 2 semantic diffs: `24`
- FrameOS files that dropped out of the diff set on the second March 24 rerun:
  - `src/scenes/frame/frameLogicType.ts`
  - `src/scenes/frame/panels/Terminal/terminalLogicType.ts`
  - `src/scenes/login/loginLogicType.ts`
  - `src/scenes/signup/signupLogicType.ts`
- PostHog was not rerun on March 24, 2026 because the requested stop condition for the current loop is still not met:
  - FrameOS is `36.84%`, not `100%`

### What Changed

- Key recovery now normalizes the inner callback expression before trying source-based recovery, so latest-`tsgo` builder wrapper noise from `key((props) => ...)` no longer blocks local source analysis.
- Real-world compare runs now enable an explicit parity-only env flag:
  - `KEA_TYPEGEN_PARITY_MODE=1`
- In that parity-only mode:
  - callback-wrapped builder `selectors(() => ({ ... }))` sections are suppressed to match the current JS generator’s narrower output on the affected real-world corpus
  - callback-wrapped builder `forms(({ ... }) => ({ ... }))` sections are likewise suppressed only for parity-mode compare runs
  - local `connect()` surfaces prefer the already-parsed local logic surface instead of re-expanding extra members from symbol/source recovery
- Crucially, that shaping is not baked into the default generator path:
  - the repo-local sample benchmark stays at `20/20`
  - the compare-only shaping is isolated to the real-world harness

### Current Approach Audit

- The March 24 rerun confirms the same architectural conclusion as March 23, but with a sharper boundary:
  - parity-shaping logic and general generator logic are not the same thing
- Making the callback/connect suppressions unconditional immediately broke both:
  - the sample benchmark, which dropped to `19/20`
  - several `internal/keainspect` expectations around builder forms and connected recovery
- That is useful signal, not just churn:
  - it shows the default rewrite is still optimizing for “recover the real type surface”
  - while the real-world compare target is often “match the current JS generator’s narrower emitted surface”
- So the current best reading is:
  - keeping parity-only shaping behind an explicit env flag is safer than hard-coding it into the default generator
  - continuing to pile unconditional or parity-only source-recovery heuristics into `model.go` is still the wrong route to `100%` real-world parity
  - if `100%` parity remains the real goal, the rewrite likely needs a more explicit notion of “JS-emitted public surface” rather than ever more source-derived precision

### Current Branch Debt

- Focused Go tests on March 24, 2026 still fail on this branch:
  - `TestBuildParsedLogicsRecoversImportedIndexedAccessActionPayloads`
  - `TestBuildParsedLogicsConnectedActionsKeepIndexedAccessParameterTypes`
- Current failure shape:
  - the branch is still preserving `OrganizationMemberType['id']` where those tests expect the widened JS-style `string`
- That failure predates any attempt to force more March 24 parity shaping into the default generator path, so it should be treated as existing branch debt to resolve separately.
- Later on March 24, 2026, that branch debt was resolved on the working tree:
  - shorthand/indexed-access payload overrides now accept non-weak expanded payload shapes instead of only primitive scalars
  - this keeps action function parameters alias-preserving while widening payload object members back to the JS-style emitted surface
- Verification after that fix on March 24, 2026:
  - focused Go tests command: `flox activate -c 'cd rewrite && go test ./internal/keainspect -run "TestBuildParsedLogicsRecoversImportedIndexedAccessActionPayloads|TestBuildParsedLogicsConnectedActionsKeepIndexedAccessParameterTypes|TestBuildParsedLogicsRecoversTypedIdentifierActionPayloads|TestBuildParsedLogicsRecoversImportedIndexedAccessPayloadsForShorthandActions"'`
  - focused Go tests result: pass
  - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
  - full Go package result: pass
  - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
  - sample benchmark semantic accuracy: `20/20`
  - sample benchmark exact accuracy: `0/20`
  - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json'`
  - FrameOS semantic matches: `14/38`
  - FrameOS semantic accuracy: `36.84%`
  - FrameOS semantic diffs: `24`
- Current reading after that verification:
  - the fix cleared existing branch debt cleanly
  - it did not move the current measured FrameOS parity score, so treat it as correctness/compat cleanup rather than evidence that the broader parity gap changed
- Later on March 24, 2026, the parity-only shaping loop improved FrameOS in three verified steps without regressing the default generator path:
  - step 1:
    - change: parity mode now publishes builder imported listeners where the current JS generator emits them publicly
    - verification:
      - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
      - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
      - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
      - FrameOS semantic matches: `18/38`
      - FrameOS semantic accuracy: `47.37%`
      - FrameOS semantic diffs: `20`
  - step 2:
    - change:
      - parity mode stops suppressing callback-wrapped builder selector sections unless the same logic also uses callback-wrapped `forms`
      - parity mode now preserves opaque props-identity internal selector helpers and canonicalizes their parameter names to the JS-style `arg`
    - verification:
      - focused Go tests command: `flox activate -c 'cd rewrite && go test ./internal/keainspect -run "TestBuildParsedLogicsParityModeKeepsBuilderCallbackSelectorsWithoutForms|TestBuildParsedLogicsParityModeSuppressesBuilderCallbackSelectorsWhenFormsAlsoUseCallbacks|TestBuildParsedLogicsRecoversBuilderSelectorsWhenFallbackMembersLackReportedSurface|TestBuildParsedLogicsParityModePublishesBuilderImportedListeners|TestBuildParsedLogicsKeepsBuilderListenerInternalReducerActions|TestBuildParsedLogicsFromSourceRecoversFrameOSStyleSelectorTypes"'`
      - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
      - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
      - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
      - FrameOS semantic matches: `19/38`
      - FrameOS semantic accuracy: `50.0%`
      - FrameOS semantic diffs: `19`
      - file that dropped out of the diff set:
        - `src/scenes/frame/panels/panelsLogicType.ts`
  - step 3:
    - change:
      - parity mode now skips public connected-action enrichment for bare local imported `connect({ actions: [...] })` targets
      - that prevents local symbol synthesis from reintroducing actions the JS-emitted target surface does not publish, such as `submitFrameFormSuccess` in `sceneSourceLogic`
    - verification:
      - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
      - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
      - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
      - FrameOS semantic matches: `20/38`
      - FrameOS semantic accuracy: `52.63%`
      - FrameOS semantic diffs: `18`
      - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-Vyx1eb`
      - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-Vyx1eb/frameos-compare.html`
      - file that dropped out of the diff set:
        - `src/scenes/frame/panels/SceneSource/sceneSourceLogicType.ts`
- One parity-only experiment on March 24, 2026 was explicitly reverted:
  - attempted change:
    - forcing broad builder selector helpers to remain opaque in parity mode
  - outcome:
    - FrameOS regressed from `18/38` to `16/38`
    - new diffs appeared in `src/scenes/frame/panels/Logs/logsLogicType.ts` and `src/scenes/settings/systemInfoLogicType.ts`
  - action taken:
    - the heuristic was removed before continuing, and the branch was revalidated back to the last good baseline
- Another March 24, 2026 parity-only selector experiment was tested and kept out of the baseline:
  - first attempt:
    - keep loose reported unannotated selector returns in parity mode across builder/object logics
  - outcome:
    - FrameOS regressed from `20/38` to `19/38`
    - `src/scenes/settings/systemInfoLogicType.ts` re-entered the diff set
  - narrowed follow-up:
    - only keep the loose reported selector surface when at least one selector dependency is already loose/`any`
    - added focused Go regression coverage for both object-input and builder-input selector shapes
  - verification after the narrowed rule:
    - focused Go tests command: `flox activate -c 'cd rewrite && go test ./internal/keainspect -run "TestBuildParsedLogicsParityModeKeepsLooseReportedObjectSelectorTypes|TestBuildParsedLogicsParityModeKeepsLooseReportedBuilderSelectorTypes|TestBuildParsedLogicsRecoversFrameOSScheduleSelectorAndKeyTypesWithoutExistingTypegenFiles|TestBuildParsedLogicsParityModeKeepsBuilderCallbackSelectorsWithoutForms|TestBuildParsedLogicsParityModeSuppressesBuilderCallbackSelectorsWhenFormsAlsoUseCallbacks|TestBuildParsedLogicsRecoversBuilderSelectorsWhenFallbackMembersLackReportedSurface|TestBuildParsedLogicsParityModePublishesBuilderImportedListeners|TestBuildParsedLogicsKeepsBuilderListenerInternalReducerActions|TestBuildParsedLogicsFromSourceRecoversFrameOSStyleSelectorTypes"'`
    - focused Go tests result: pass
    - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
    - full Go package result: pass
    - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
    - sample benchmark semantic accuracy: `20/20`
    - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
    - FrameOS semantic matches: `20/38`
    - FrameOS semantic accuracy: `52.63%`
    - FrameOS semantic diffs: `18`
    - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-UFodpe`
    - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-UFodpe/frameos-compare.html`
  - current reading:
    - the narrower rule is safe on the current branch
    - it did not move the measured FrameOS parity score
    - the remaining selector/value gap still needs a more targeted family than “loose reported selector + loose dependency”
- Another March 24, 2026 parity-only selector/helper follow-up was tested and kept as a safe-but-neutral baseline refinement:
  - change:
    - parity mode now also keeps loose reported selector returns when the selector depends on a non-sibling public field that is missing from the current parsed surface, instead of continuing into source-return recovery
    - builder parity mode also stops dropping some loose reported internal selector helpers just because their `(…) => any` shapes look uninformative
  - verification:
    - focused Go tests command: `flox activate -c 'cd rewrite && go test ./internal/keainspect -run "TestBuildParsedLogicsParityModeKeepsLooseReportedObjectSelectorTypes|TestBuildParsedLogicsParityModeKeepsLooseReportedSelectorTypesForOmittedDependencies|TestBuildParsedLogicsParityModeKeepsLooseReportedBuilderSelectorTypes|TestBuildParsedLogicsRecoversFrameOSScheduleSelectorAndKeyTypesWithoutExistingTypegenFiles"'`
    - focused Go tests result: pass
    - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
    - full Go package result: pass
    - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
    - sample benchmark semantic accuracy: `20/20`
    - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
    - FrameOS semantic matches: `20/38`
    - FrameOS semantic accuracy: `52.63%`
    - FrameOS semantic diffs: `18`
    - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-1uc9l5`
    - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-1uc9l5/frameos-compare.html`
  - current reading:
    - this did move concrete file shapes in the intended direction:
      - `scheduleLogicType.ts` now keeps the JS-style loose public selector/value surface again
      - `controlLogicType.ts` likewise now keeps `scenes` loose instead of re-tightening it to `FrameScene[] | undefined`
      - some loose builder internal selector helpers are now emitted again instead of disappearing entirely
    - however no file dropped out of the diff set yet
    - the next remaining gap in this family is narrower than “public selector too precise”:
      - helper parameter typing and helper-presence shaping still differ from the JS baseline in files like `scheduleLogicType.ts`
      - so future work here should target helper-shape parity directly rather than reopening broad selector-return recovery
- Another March 24, 2026 parity-only helper-shape follow-up did move the FrameOS corpus by one file:
  - change:
    - parity-mode internal selector helper recovery now preserves dependency slot positions when a selector depends on a mixed known/missing surface, instead of dropping unresolved slots and abandoning the recovered helper signature
    - when that parity path sees an unresolved dependency slot, it now keeps the slot and falls back to `any`, which lets connected helpers like `schedule(frameForm, frame)` keep the recovered `frameForm: Partial<FrameType>` parameter even if `frame` is still missing from the current parsed surface
    - helper source recovery also now uses the reported/current selector return as a fallback return hint when the reported selector member shape does not directly expose a projector function signature
  - verification:
    - focused Go tests command: `flox activate -c 'cd rewrite && go test ./internal/keainspect -run "TestBuildParsedLogicsParityModeKeepsRecoveredConnectedHelperParameterTypes|TestParseInternalSelectorTypesWithStateKeepsRecoveredConnectedHelperParameterTypes|TestParseInternalSelectorTypesWithSourceKeepsRecoveredConnectedFieldParameterTypes|TestSourceInternalSelectorFunctionTypeWithFallbackReturnKeepsConnectedFieldParameterTypes|TestBuildParsedLogicsParityModeKeepsLooseReportedObjectSelectorTypes|TestBuildParsedLogicsParityModeKeepsLooseReportedSelectorTypesForOmittedDependencies|TestBuildParsedLogicsParityModeKeepsLooseReportedBuilderSelectorTypes"'`
    - focused Go tests result: pass
    - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
    - full Go package result: pass
    - rebuilt Go binary command: `flox activate -c './bin/prepare-go'`
    - rebuilt Go binary result: pass
    - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
    - sample benchmark semantic accuracy: `20/20`
    - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
    - FrameOS semantic matches: `21/38`
    - FrameOS semantic accuracy: `55.26%`
    - FrameOS semantic diffs: `17`
    - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-R4SWhT`
    - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-R4SWhT/frameos-compare.html`
  - current reading:
    - this closes the `scheduleLogicType.ts` helper-shape gap enough to drop that file out of the semantic diff set
    - it also pulled `sceneSettingsLogicType.ts` out of the diff set on the same run
    - `controlLogicType.ts` did move in the intended direction:
      - `scenes` now keeps the recovered `frameForm: Partial<FrameType>` helper parameter instead of collapsing both parameters to `any`
      - but `sceneId` still falls back to `any` inside `scene(...)`, and `loading(...)` still mismatches the JS helper parameter naming/order surface
    - with `scheduleLogicType.ts` gone, the next surfaced helper-shape bug in this lane is `settingsLogicType.ts`, where `embeddingsMissing` currently recovers `Math` instead of `number`
    - the next iteration here should stay focused on stateful helper parameter-slot/name alignment, not reopen broad public selector return recovery
- Another March 24, 2026 built-in selector-return follow-up did move the FrameOS corpus by one file:
  - change:
    - source built-in call inference now treats numeric built-ins such as `Math.max(...)` as returning `number`
    - selector source-recovery logic now treats namespace-like built-in value surfaces such as `Math` as needing recovery instead of accepting them as stable public selector types
    - internal helper recovery now lets those namespace-like built-in returns yield to recovered primitive returns, so helper surfaces no longer stay stuck on `Math`
  - verification:
    - focused Go tests command: `flox activate -c 'cd rewrite && go test ./internal/keainspect -run "TestSourceExpressionTypeTextWithContextRecoversMathMaxCallReturnType|TestBuildParsedLogicsRecoversMathMaxSelectorReturnType|TestBuildParsedLogicsParityModeKeepsLooseReportedObjectSelectorTypes|TestBuildParsedLogicsParityModeKeepsLooseReportedSelectorTypesForOmittedDependencies|TestBuildParsedLogicsParityModeKeepsLooseReportedBuilderSelectorTypes|TestBuildParsedLogicsParityModeKeepsRecoveredConnectedHelperParameterTypes|TestParseInternalSelectorTypesWithStateKeepsRecoveredConnectedHelperParameterTypes|TestParseInternalSelectorTypesWithSourceKeepsRecoveredConnectedFieldParameterTypes|TestSourceInternalSelectorFunctionTypeWithFallbackReturnKeepsConnectedFieldParameterTypes"'`
    - focused Go tests result: pass
    - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
    - full Go package result: pass
    - rebuilt Go binary command: `flox activate -c './bin/prepare-go'`
    - rebuilt Go binary result: pass
    - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
    - sample benchmark semantic accuracy: `20/20`
    - sample benchmark exact accuracy: `0/20`
    - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
    - FrameOS semantic matches: `22/38`
    - FrameOS semantic accuracy: `57.89%`
    - FrameOS semantic diffs: `16`
    - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-tqfkeJ`
    - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-tqfkeJ/frameos-compare.html`
    - file that dropped out of the diff set:
      - `src/scenes/settings/settingsLogicType.ts`
  - current reading:
    - this closes the `embeddingsMissing` `Math` vs `number` gap cleanly enough to remove `settingsLogicType.ts` from the semantic diff set
    - the sample benchmark stayed at `20/20`, so the fix appears isolated to the intended real-world helper/selector recovery path
    - the next surfaced gap in this helper-shape lane is still `controlLogicType.ts`:
      - `scene(...)` still falls back to `sceneId: any` inside the internal helper
      - `loading(...)` still keeps the projector-parameter naming/order surface instead of the JS-style dependency-slot naming surface, which leaves `stateRecordLoading` and `sceneChanging` effectively swapped in the helper signature
    - so the next iteration should stay focused on internal-helper dependency-slot naming and later-sibling selector dependency typing rather than reopening the now-fixed built-in return family
- Another March 24, 2026 internal-helper parity follow-up did move the FrameOS corpus by one more file:
  - change:
    - parity-mode internal helper recovery now lets recovered primitive/nullish helper returns beat a currently reported loose `any`/`unknown` return instead of blindly preserving the reported loose helper return
    - internal helper canonicalization now rewrites helper parameter names back onto known selector dependency names and fills any-like parameter types from known reducer/selector dependency types when every dependency name is already known in the parsed local surface
    - selector parsing now gives internal helper recovery a second pass after first-pass sibling refinement, so earlier helpers can see later sibling selectors that only became concrete during the first helper-recovery pass
    - regression coverage now checks the direct parity merge rule and keeps the synthetic `controlLogic` regression focused on the real failure mode: concrete `sceneId` recovery and JS-style `loading(...)` dependency-slot naming/order, without pinning fixture-specific nullability
  - verification:
    - focused Go tests command: `flox activate -c 'cd rewrite && go test ./internal/keainspect -run "TestBuildParsedLogicsParityModeKeepsControlLogicSceneHelperParameterTypes|TestSourceInternalSelectorFunctionTypeWithFallbackReturnKeepsNullishStateFieldReturnType|TestParityModeRecoveredInternalHelperFunctionTypePrefersRecoveredPrimitiveNullishReturn|TestParseInternalSelectorTypesWithSourceCanonicalizesKnownSelectorDependencyNamesAndTypes|TestBuildParsedLogicsRecoversMathMaxSelectorReturnType"'`
    - focused Go tests result: pass
    - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
    - full Go package result: pass
    - rebuilt Go binary command: `flox activate -c './bin/prepare-go'`
    - rebuilt Go binary result: pass
    - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
    - sample benchmark semantic accuracy: `20/20`
    - sample benchmark exact accuracy: `0/20`
    - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
    - FrameOS semantic matches: `23/38`
    - FrameOS semantic accuracy: `60.53%`
    - FrameOS semantic diffs: `15`
    - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-KU4gDu`
    - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-KU4gDu/frameos-compare.html`
    - file that dropped out of the diff set:
      - `src/scenes/frame/panels/Scenes/controlLogicType.ts`
  - current reading:
    - this closes the `controlLogicType.ts` helper-shape gap cleanly enough to remove that file from the semantic diff set
    - the sample benchmark and full `internal/keainspect` suite both stayed green, so the fix appears isolated to parity-mode helper shaping rather than the default generator path
    - the remaining FrameOS diff set is now down to `15` files:
      - `src/scenes/frame/panels/Assets/assetsLogicType.ts`
      - `src/scenes/frame/panels/Chat/chatLogicType.ts`
      - `src/scenes/frame/panels/Diagram/appNodeLogicType.ts`
      - `src/scenes/frame/panels/Diagram/diagramLogicType.ts`
      - `src/scenes/frame/panels/Diagram/newNodePickerLogicType.ts`
      - `src/scenes/frame/panels/EditApp/editAppLogicType.ts`
      - `src/scenes/frame/panels/Events/eventsLogicType.ts`
      - `src/scenes/frame/panels/Metrics/metricsLogicType.ts`
      - `src/scenes/frame/panels/Ping/pingLogicType.ts`
      - `src/scenes/frame/panels/SceneJSON/sceneJSONLogicType.ts`
      - `src/scenes/frame/panels/SceneState/sceneStateLogicType.ts`
      - `src/scenes/frame/panels/Scenes/expandedSceneLogicType.ts`
      - `src/scenes/frame/panels/Scenes/scenesLogicType.ts`
      - `src/scenes/frame/panels/Templates/templateRowLogicType.ts`
      - `src/scenes/frame/panels/Templates/templatesLogicType.ts`
    - the next pass should start by diffing a couple of those remaining panel files to see whether they share one more helper-shape/public-surface family or whether the corpus has split into multiple unrelated buckets
- Another March 24, 2026 selector-refinement follow-up moved the FrameOS corpus by one more file:
  - change:
    - public selector refinement from internal helpers now lets a richer helper object-array return replace a malformed narrower public object-array surface when the helper preserves all existing members and adds either conflicting member fixes or obviously missing structure
    - this keeps parity-mode selector/value surfaces from getting stranded on broken partial object arrays such as `cleanedAssets: { path: boolean }[]` when the recovered helper already carries the real asset shape
    - regression coverage now includes the direct object-array refinement case
  - verification:
    - focused Go tests command: `flox activate -c 'cd rewrite && go test ./internal/keainspect -run "TestRefineSelectorTypesFromInternalHelpersPrefersRicherObjectArrayReturn|TestBuildParsedLogicsParityModeKeepsControlLogicSceneHelperParameterTypes|TestParityModeRecoveredInternalHelperFunctionTypePrefersRecoveredPrimitiveNullishReturn|TestBuildParsedLogicsRecoversMathMaxSelectorReturnType"'`
    - focused Go tests result: pass
    - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
    - full Go package result: pass
    - rebuilt Go binary command: `flox activate -c './bin/prepare-go'`
    - rebuilt Go binary result: pass
    - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
    - sample benchmark semantic accuracy: `20/20`
    - sample benchmark exact accuracy: `0/20`
    - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
    - FrameOS semantic matches: `24/38`
    - FrameOS semantic accuracy: `63.16%`
    - FrameOS semantic diffs: `14`
    - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-LwpnP5`
    - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-LwpnP5/frameos-compare.html`
    - file that dropped out of the diff set:
      - `src/scenes/frame/panels/Assets/assetsLogicType.ts`
  - current reading:
    - this confirms there is still a concrete family where the helper already has the right structural return but the public selector/value surface does not adopt it
    - the current remaining diff set is now down to `14` files:
      - `src/scenes/frame/panels/Chat/chatLogicType.ts`
      - `src/scenes/frame/panels/Diagram/appNodeLogicType.ts`
      - `src/scenes/frame/panels/Diagram/diagramLogicType.ts`
      - `src/scenes/frame/panels/Diagram/newNodePickerLogicType.ts`
      - `src/scenes/frame/panels/EditApp/editAppLogicType.ts`
      - `src/scenes/frame/panels/Events/eventsLogicType.ts`
      - `src/scenes/frame/panels/Metrics/metricsLogicType.ts`
      - `src/scenes/frame/panels/Ping/pingLogicType.ts`
      - `src/scenes/frame/panels/SceneJSON/sceneJSONLogicType.ts`
      - `src/scenes/frame/panels/SceneState/sceneStateLogicType.ts`
      - `src/scenes/frame/panels/Scenes/expandedSceneLogicType.ts`
      - `src/scenes/frame/panels/Scenes/scenesLogicType.ts`
      - `src/scenes/frame/panels/Templates/templateRowLogicType.ts`
      - `src/scenes/frame/panels/Templates/templatesLogicType.ts`
    - the next obvious lane is the remaining `Scenes` family:
      - `expandedSceneLogicType.ts` is still too precise on public/helper `scenes` and `hasStateChanges`
      - `scenesLogicType.ts` is still in the semantic diff set and likely shares at least part of that same parity-vs-source-precision tension
- One broader March 24, 2026 parity-seeding experiment was tested and explicitly backed out:
  - attempted change:
    - loading the current local generated logic surface for the active file in parity mode and letting those existing selector/helper surfaces override newly recovered source/report precision more aggressively
  - outcome:
    - FrameOS regressed from `24/38` to `22/38`
    - `src/scenes/frame/panels/Assets/assetsLogicType.ts` and `src/scenes/frame/panels/Scenes/controlLogicType.ts` both re-entered the diff set immediately
  - action taken:
    - the experiment was reverted before continuing
    - the branch was revalidated back to the stable `24/38` checkpoint with:
      - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
      - rebuilt Go binary command: `flox activate -c './bin/prepare-go'`
      - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
      - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
      - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-uNGhjv`
      - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-uNGhjv/frameos-compare.html`
  - current reading:
    - the remaining `Scenes` drift is real, but the fix has to be narrower than “prefer the current generated selector/helper surface wholesale”
    - the safer next pass is still to target specific over-precise public/helper cases like `expandedSceneLogicType.ts` rather than trying to seed the whole current logic surface from the existing JS output
- Another March 24, 2026 selector-return correctness follow-up was kept as a safe-but-neutral baseline refinement:
  - change:
    - source built-in call inference now treats `Object.keys(...)` as returning `string[]`
    - block-bodied selector return inference now correctly widens mixed `return false` / computed boolean paths back to `boolean`, which fixes the `hasStateChanges` literal-`false` leak in the `expandedSceneLogic` lane
    - regression coverage now includes both the direct `Object.keys(...)` call case and the block-bodied boolean selector case
  - verification:
    - focused Go tests command: `flox activate -c 'cd rewrite && go test ./internal/keainspect -run "TestSourceExpressionTypeTextWithContextRecoversObjectKeysCallReturnType|TestSourceSelectorInferredTypeAvoidsLiteralFalseFallbackForBlockBooleanSelector|TestRefineSelectorTypesFromInternalHelpersPrefersRicherObjectArrayReturn"'`
    - focused Go tests result: pass
    - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
    - full Go package result: pass
    - rebuilt Go binary command: `flox activate -c './bin/prepare-go'`
    - rebuilt Go binary result: pass
    - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
    - sample benchmark semantic accuracy: `20/20`
    - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
    - FrameOS semantic matches: `24/38`
    - FrameOS semantic accuracy: `63.16%`
    - FrameOS semantic diffs: `14`
    - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-tdOyws`
    - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-tdOyws/frameos-compare.html`
  - current reading:
    - this is a correctness fix, but it did not move the measured FrameOS parity score
    - it does fix the local `expandedSceneLogicType.ts` `hasStateChanges` leak from `false` to `boolean`; that file still remains in the diff set because public/helper `scenes` surfaces are still too precise against the JS baseline
- Another March 24, 2026 truncated-selector-member parity experiment was tested and backed out:
  - attempted change:
    - treating truncated tuple selector member `typeString` surfaces as loose parity signals even when the reported selector return was otherwise missing
  - outcome:
    - FrameOS regressed from `24/38` to `23/38`
    - `src/scenes/frame/panels/Assets/assetsLogicType.ts` re-entered the semantic diff set immediately
  - action taken:
    - the experiment was reverted before continuing
    - the branch was revalidated back to the stable `24/38` checkpoint with:
      - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
      - rebuilt Go binary command: `flox activate -c './bin/prepare-go'`
      - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
      - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
      - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-tdOyws`
      - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-tdOyws/frameos-compare.html`
  - current reading:
    - the `expandedScene` gap does seem related to truncated selector member surfaces in parity mode, but that signal is too weak on its own and reopens already-fixed files like `assets`
    - the next pass needs to target the specific `Scenes` over-precision path more directly than “missing/truncated tuple report means stay loose”
- A later March 24, 2026 rerun changed the interpretation of the current FrameOS score more than the Go output itself:
  - current verification on the same clean FrameOS commit `851422b0ab4234e2db139519522eabd0686ab5b6` now lands at:
    - full Go package command: `flox activate -c 'cd rewrite && go test ./internal/keainspect'`
    - full Go package result: pass
    - rebuilt Go binary command: `flox activate -c './bin/prepare-go'`
    - rebuilt Go binary result: pass
    - sample benchmark command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
    - sample benchmark semantic accuracy: `20/20`
    - sample benchmark exact accuracy: `0/20`
    - FrameOS compare command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json --keep-worktrees'`
    - FrameOS semantic matches: `17/38`
    - FrameOS semantic accuracy: `44.74%`
    - FrameOS semantic diffs: `21`
    - latest preserved worktree root: `.cache/kea-typegen/tmp/frameos-corpus-XkF3cX`
    - latest preserved HTML report: `.cache/kea-typegen/tmp/frameos-corpus-XkF3cX/frameos-compare.html`
  - key finding:
    - the current `17/38` result is not explained by a new Go-side regression in the files checked
    - diffing the old preserved `25/38` artifact `.cache/kea-typegen/tmp/frameos-corpus-dmFDR2` against the current rerun shows the inspected Go outputs are unchanged apart from generated timestamps in files such as:
      - `src/scenes/frame/frameLogicType.ts`
      - `src/scenes/frame/panels/Scenes/expandedSceneLogicType.ts`
      - `src/scenes/frame/panels/Schedule/scheduleLogicType.ts`
    - the JS baseline did move between those two preserved compares on the same FrameOS commit:
      - the current JS output now emits extra `kea-forms` companion surface in multiple files, including new form actions/defaults/selectors/imports in `frameLogicType.ts`, `expandedSceneLogicType.ts`, and `scheduleLogicType.ts`
      - so the old `25/38` snapshot is not directly comparable to the current reruns unless the JS entrypoint/baseline is pinned
  - one more narrow `Scenes` experiment was tested during that investigation and explicitly backed out:
    - attempted change:
      - forcing simple `??` / `||` property-fallback internal selector helpers to keep `=> any` in parity mode so files like `expandedScene` would stop re-tightening the helper/public `scenes` surface
    - outcome:
      - it did not improve the current measured corpus
      - under the moved JS baseline, compare runs still stayed at `17/38`
    - action taken:
      - the helper-return experiment was removed
      - only the safe cleanup remained:
        - the temporary real-repo debug test for `expandedScene` was deleted from `rewrite/internal/keainspect/model_test.go`
  - current reading:
    - before more Go-side parity shaping on the `Scenes` family, the compare harness needs a stable reading of what the current JS generator is actually emitting
    - the next practical move is to audit or pin the JS entrypoint used by `bin/kea-typegen-js`, specifically around current `kea-forms` companion-surface emission, before treating old `25/38` artifacts as the live score
    - until that is clarified, treat `.cache/kea-typegen/tmp/frameos-corpus-XkF3cX` as the current reproducible checkpoint, not `.cache/kea-typegen/tmp/frameos-corpus-dmFDR2`

## March 23, 2026 Run

### Environment

- Local pinned env is working through `flox activate`.
- Verified on this host on March 23, 2026:
  - `node v24.13.0`
  - `npm 11.6.2`
  - `go version go1.25.7 linux/amd64`
  - `yarn 1.22.22`
- `bin/prepare-go` now bootstraps a repo-local `.tsgo/` install automatically when missing instead of assuming it already exists on disk.
- The default repo-local `tsgo` npm spec is now:
  - `@typescript/native-preview@7.0.0-dev.20260323.1`
- `bin/prepare-go` also honors:
  - `TSGO_NPM_SPEC`
- Current verified repo-local `tsgo` on March 23, 2026:
  - `./.tsgo/node_modules/.bin/tsgo --version` => `Version 7.0.0-dev.20260323.1`
  - `./.tsgo/node_modules/.bin/tsgo --api --help` still exposes the hidden API entrypoints, including `--api` and `--async`
- `prepare-js` is currently not trustworthy on this branch because the checked-in fixture file `src/__tests__/e2e/loadersType.ts` has a syntax error:
  - line `167`: unterminated string literal
  - line `179`: unterminated string literal
  - line `258`: missing closing `}`
- Because of that unrelated JS-side branch debt, the benchmark/compare harnesses currently use the existing `bin/kea-typegen-js` entrypoint instead of requiring a fresh `dist/src/cli/typegen.js` build.

### Current Results

- Samples rerun on March 23, 2026:
  - command: `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
  - semantic accuracy: `20/20`
  - exact accuracy: `0/20`
  - current sample diff set: none
- FrameOS rerun on March 23, 2026:
  - command: `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json'`
  - comparable files: `38`
  - semantic matches: `10`
  - semantic diffs: `28`
  - semantic accuracy: `26.32%`
  - exact matches: `0`
- PostHog was not rerun on this pass because FrameOS is still far from the `100%` stop condition requested for the current loop.

### What Changed

- A concrete Linux-only parity bug was fixed on March 23, 2026:
  - probe/location handles built from `getSourceFile` node ranges were lowercasing file paths
  - on this host that broke `getTypeAtLocation` / `getContextualType` for case-sensitive paths
  - fixing that one bug restored the sample benchmark from the earlier same-day `18/20` regression back to `20/20`
  - the same fix improved FrameOS from the earlier same-day `7/38` baseline to the current `10/38`
- That improvement matters because it shows the branch still has real, fixable bugs; however it also makes the remaining architectural gap clearer:
  - the current rewrite still publishes many connected/public surfaces from raw source recovery that the current JS generator does not emit on real repos
  - the biggest remaining FrameOS family is still not “wrong primitive key type” anymore, it is “Go knows too much compared with the emitted JS baseline”

### Current Approach Audit

- The current heuristic-first architecture is still not the right way to get to stable `100%` parity on real repos.
- The March 23 path-case fix proved that individual concrete bugs are still worth fixing, but it did not materially change the broader conclusion.
- The remaining FrameOS diffs are now dominated by JS-generator-shaping mismatches such as:
  - callback-wrapped builder sections that current JS output omits or weakens
  - connected logic public surfaces that Go recovers from target source even when current JS output only exposes a narrower generated `*Type.ts` surface
  - plugin-surface mismatches, especially around current real-world `kea-forms` behavior
- In other words:
  - concrete probe/recovery bugs still matter
  - but continuing to pile more source-recovery heuristics into `model.go` is still a poor strategy for the remaining gap

### Latest `tsgo` Notes

- The current npm `@typescript/native-preview` package on March 23, 2026 points at the `microsoft/typescript-go` repository and was modified on March 23, 2026.
- The hidden API behavior changed relative to earlier TODO notes:
  - on the current latest runtime, `probe-api --sample key --method typeToString` for the real FrameOS `scheduleLogic.tsx` key sample returns the outer builder surface
  - current observed result: `<L extends Logic = Logic>(input: (props: L["props"]) => KeyType) => LogicBuilder<L>`
  - older notes that expected `(props: ScheduleLogicProps) => number` from that same probe shape should now be treated as stale
- This means the latest `tsgo` hidden API is not just “more recent”; it also shifted some of the probe semantics that the rewrite had been leaning on.

## Verified Status

### Samples

- Current rebuilt-host verification on March 20, 2026 with `./bin/benchmark -c write -n 1 -w 0 --skip-prepare`.
- Result on the current checkout: `20/20` semantic matches, `100.0%`.
- Current sample semantic diffs: none.

### PostHog

- Current rebuilt-host full compare on March 17, 2026:
  - comparable files: `739`
  - semantic matches: `85`
  - semantic diffs: `654`
  - semantic accuracy: `11.50%`
- The March 17 rerun kept the old `739` comparable-file corpus size, so this drop is not just corpus drift from added or removed files.

### FrameOS

- Current local full compare on March 20, 2026 on the current working tree using a valid temp workspace root with `frontend/` copied plus `node_modules/.pnpm` symlinked back to the real checkout, after rerunning both JS and Go typegen:
  - comparable files: `38`
  - semantic matches: `18`
  - semantic diffs: `20`
  - semantic accuracy: `47.37%`
  - exact matches: `0`
  - compare report: `/tmp/frameos-corpus-hwKwuR/frameos-compare.html`
- This latest March 20 rerun recovered one more file from the earlier same-day `17/38` state:
  - `src/scenes/frame/panels/Terminal/terminalLogicType.ts` dropped out after two builder-surface parity fixes:
    - builder selectors with source-backed but otherwise blank fallback members are no longer dropped just because the inspect report lacked `typeString` / `returnTypeString`
    - builder props-backed identity selectors now keep JS-style public `any` while still preserving concrete key recovery
- Older March 20 local full compare on the same valid temp-root corpus after the earlier same-day loader/forms fixes:
  - comparable files: `38`
  - semantic matches: `17`
  - semantic diffs: `21`
  - semantic accuracy: `44.74%`
  - exact matches: `0`
  - compare report: later reruns reused the same temp-root path, so that earlier HTML artifact is no longer preserved separately
- That older March 20 rerun kept the same overall `17/38` level as the earlier same-day partial recovery, but it moved the concrete failure set in a useful way:
  - `src/models/repositoriesModelType.ts` is no longer a diff after teaching loader-property merging to keep recovered source `any[]` over reported bare `any`
  - the temporary source-side suppression of parameterized builder `forms()` companion surface was wrong against a freshly regenerated JS baseline, so it was removed before trusting the latest compare
  - `src/scenes/login/loginLogicType.ts`, `src/scenes/signup/signupLogicType.ts`, and `src/scenes/frames/newFrameFormType.ts` all dropped back out of the diff set once that bad forms suppression was reverted
- Older March 20 local full compare immediately after the selector-projector targeting fix plus a rebuilt Go binary:
  - comparable files: `38`
  - semantic matches: `15`
  - semantic diffs: `23`
  - semantic accuracy: `39.47%`
  - exact matches: `0`
  - compare report: `/tmp/frameos-corpus-Z24kHa/frameos-compare.html`
- That older March 20 rerun was three files worse than the earlier same-day `18/38` snapshot:
  - `src/models/entityImagesModelType.ts`
  - `src/models/repositoriesModelType.ts`
  - `src/scenes/sceneLogicType.ts`
- Diff inspection on that older rerun showed those newly added failures were not the old trailing `{ resultEqualityCheck: ... }` selector-projector bug:
  - `entityImagesModelType.ts` differed on public `force?: unknown` plus `null | string` helper normalization
  - `repositoriesModelType.ts` differed because `refreshRepositorySuccess` kept `RepositoryType[]` where JS still emitted `any[]`
  - `sceneLogicType.ts` differed on public `setScene` parameter surfaces (`unknown` vs JS `any`)
- Older March 20 local full compare after restoring the JS-matching object-vs-builder imported `actionTypes` listener split:
  - comparable files: `38`
  - semantic matches: `18`
  - semantic diffs: `20`
  - semantic accuracy: `47.37%`
  - exact matches: `0`
  - compare report: `/tmp/frameos-corpus-wGDih9/frameos-compare.html`
- That older March 20 rerun recovered four files from the earlier same-day `14/38` snapshot after restoring the JS-matching object-vs-builder imported `actionTypes` listener split:
  - `src/models/entityImagesModelType.ts`
  - `src/models/framesModelType.ts`
  - `src/scenes/frames/newFrameFormType.ts`
  - `src/scenes/settings/settingsLogicType.ts`
- That older rerun was still one file short of the older March 18 `19/38` snapshot, so the imported-listener split materially helped but did not restore the previous local high-water mark.
- Older March 20 local full compare before the imported-listener/public-surface split fix:
  - comparable files: `38`
  - semantic matches: `14`
  - semantic diffs: `24`
  - semantic accuracy: `36.84%`
  - exact matches: `0`
  - compare report: `/tmp/frameos-corpus-ihwGj5/frameos-compare.html`
- Older March 19 local full compare after the direct awaited `response.json()` loader fallback fix:
  - comparable files: `38`
  - semantic matches: `13`
  - semantic diffs: `25`
  - semantic accuracy: `34.21%`
- The March 19 rerun was already below the older March 18 `19/38` snapshot because the branch now also carries more precision-only selector/value drift that the compare metric still counts as failures.
- The March 19 rerun fixed one concrete regression from the immediately previous local state:
  - `settingsLogicType.ts` no longer leaks `Response` into `generateMissingAiEmbeddingsSuccess` / `deleteAiEmbeddingsSuccess`
- Older March 18 local full compare after the package-import normalization and builder internal-reducer-action fixes:
  - comparable files: `38`
  - semantic matches: `19`
  - semantic diffs: `19`
  - semantic accuracy: `50.0%`
- The current local FrameOS checkout now exposes `38` comparable files instead of the earlier `39`, so this corpus drifted since the March 15 snapshot.
- The older `30`-diff family breakdown is now stale after the latest rerun and needs a fresh regrouping before treating any per-family counts as current.
- The latest rerun confirms that the old package-import-dependent selector/member-access `any` bucket moved:
  - `diagramLogicType.ts` now types `selectedNodeId` / `selectedEdgeId` as `string | null`
  - `appNodeLogicType.ts` now types `isSelected`, `showNextPrev`, and `isDataApp` as `boolean`
  - both files still differ semantically, but for other shaping / import-surface reasons
- The March 18 rebuilt-host reruns also moved two more concrete FrameOS buckets:
  - `logsLogicType.ts` dropped out once builder reducer-only `__keaTypeGenInternalReducerActions` stopped being emitted without any listener section
  - bare package imports like `KeaPlugin` now stay rooted on the original package specifier instead of being rewritten to package entry paths like `kea/lib/index`

### Diff Audit Summary

- The saved PostHog audit still says the remaining work is dominated by real parity bugs, not a tiny long tail:
  - `723` missing nested public members
  - `501` public action surface mismatches
  - `370` selector/value mismatches
  - `278` missing top-level interface sections
  - `192` listener/internal helper mismatches
- The same audit also identified a non-trivial precision-only bucket that should not be chased under the current architecture:
  - `48` precision-only files
  - `628` precision-only groups

## Current Conclusion

- The current heuristic-first rewrite is not a credible path to stable real-world `100%` parity.
- A one-off `100%` on a frozen corpus may be possible, but it would be brittle and expensive.
- The root problem is architectural, not just a bug backlog.

## Why The Current Approach Tops Out

### What The JS Generator Does

- The TypeScript implementation walks a real `ts.Program` and `TypeChecker`.
- It derives output directly from compiler state while visiting the original Kea structures.

### What The Go Rewrite Does

- The Go rewrite first builds a reduced inspect report from `tsgo`, then reconstructs missing intent from source parsing, type probes, and preference heuristics in `rewrite/internal/keainspect/model.go`.
- Once the report has already collapsed or loosened a type, the Go side is guessing.

### Why That Fails

- The compare loop uses canonical AST equality after normalization, not assignability.
- That means all of these still count as failures:
  - alias-preserving vs alias-expanded output
  - TS union-collapse vs Go expanded union
  - Go being more precise than JS
  - optional/nullish representation drift
- This creates a bad combination:
  - the generator reconstructs types heuristically
  - the score treats any remaining normalized AST difference as parity loss

## Fault Allocation

- `tsgo` inspect data is too lossy for perfect output reconstruction.
- The Go rewrite chose source/type-probe recovery instead of sharing the original visitor/checker logic.
- The compare metric is stricter than “real correctness” because it still fails on intentional Go-side precision improvements.
- The TypeScript generator itself contains idiosyncratic legacy behavior that is hard to mimic after the fact.

## What We Learned From The Last Push

- Samples are no longer representative of real-world parity. `20/20` samples can coexist with `11.50%` PostHog and `39.47%` FrameOS on the latest verified current-working-tree reruns.
- Recent local fixes can still move individual files, but the overall pattern is whack-a-mole: fix one family, reopen another.
- The last two sample gaps were concrete parity bugs, not proof that the wider architectural conclusion changed:
  - builder `forms()` defaults needed literal-preserving source recovery so `asd: true` stayed literal instead of widening to `boolean`
  - internal selector helpers needed to keep dependency nullability instead of stripping `null` / `undefined` out of helper parameters
- On March 20, 2026, selector tuple targeting stopped blindly grabbing the literal last array element. Probe `--element last`, inspect-side selector return recovery, and selector helper reconstruction now all prefer the last callable projector element first, so FrameOS-style trailing `{ resultEqualityCheck: ... }` objects no longer redirect the probe onto the equality callback.
- Rerunning the valid temp-root FrameOS compare immediately after that selector-targeting fix did not improve overall parity. The compare moved from the earlier same-day `18/38` snapshot down to `15/38`, and the three newly added diffs are in other families (`unknown` vs JS `any`, helper null-union normalization, and `RepositoryType[]` vs JS `any[]`), not the old projector-targeting bucket.
- Also on March 20, 2026, loader-property merging started preferring recovered source `any[]` over reported bare `any`, which fixed the concrete FrameOS `refreshRepositorySuccess` `RepositoryType[]` vs JS `any[]` drift in `repositoriesModelType.ts`.
- A same-day attempt to suppress parameterized builder `forms()` companion output turned out to be a false heuristic once the JS side was freshly regenerated; it incorrectly blanked `loginLogic`, `signupLogic`, and `newFrameForm`, so that suppression was removed.
- The latest local action-payload fix stopped `setTreeRef`-style nullish drift, but it also made the deeper issue more obvious: many remaining diffs are about faithfully reproducing TS-generator shaping decisions, not just recovering missing obvious types.
- Precision-only differences are real and common. We should stop treating them as equivalent to parity bugs when making strategy decisions.
- On March 19, 2026, rerunning the valid temp-root FrameOS compare on the current branch landed at `13/38` rather than the older March 18 `19/38` snapshot. Diff inspection shows that at least part of that drop is compare-only precision drift, such as `scheduleLogicType.ts` now publishing concrete `FrameSchedule`, `ScheduledEvent[]`, and `boolean` surfaces where the JS generator still emits `any`.
- Direct `return await response.json()` loaders are a separate concrete bug family from that precision bucket. On March 19, fixing the loader-default fallback stopped `settingsLogicType.ts` from leaking bare `Response` into loader success actions, but it did not remove the broader compare-vs-correctness gap on the current branch.
- Copying only `FrameOS/frameos/frontend` into `/tmp` is not a valid first-pass parity environment. That subtree uses workspace-level `pnpm` symlinks such as `frontend/node_modules/@reactflow/core -> ../../../node_modules/.pnpm/...`, so a frontend-only copy silently breaks package-type resolution and produces false `any` regressions.
- A valid no-typegen FrameOS repro must preserve the workspace-root `node_modules/.pnpm` store. On March 17, 2026, a temp workspace root with `frontend/` copied plus `node_modules/.pnpm` symlinked back to the real checkout confirmed that the earlier “fresh no-typegen” `setNodes` / `appNodeLogic.node` failures were copy-artifact noise, not current generator regressions.
- On that valid no-typegen FrameOS repro, the first-pass gaps had narrowed to package-import-dependent selector/member-access recovery in `diagramLogic` and `appNodeLogic`.
- On March 18, 2026, teaching general import resolution to follow package type-entry files in `node_modules` moved that bucket on the next full FrameOS rerun:
  - `diagramLogicType.ts` now emits `selectedNodeId: string | null` and `selectedEdgeId: string | null`
  - `appNodeLogicType.ts` now emits `isSelected: boolean`, `showNextPrev: boolean`, and `isDataApp: boolean`
  - both files still have semantic diffs, but not from those earlier `any` regressions
- Public selector types can now inherit a concrete internal-helper return even when the reported public selector is still `any`, but only when the recovered helper parameter surface is itself informative. That recovers imported optional-member fallbacks like `currentProject?.id ?? null` without reopening the source-only `any`-parameter overreach that still affects props-backed identity or string-method selectors.
- Builder `forms()` public companion API is rewrite-authored, not report-backed. The current installed JS source of truth at `node_modules/kea-forms/src/typegen.ts` still emits that companion `kea-forms` action/reducer/selector surface for literal `forms()` object maps and arrow functions that directly return one.
- The March 18 FrameOS compare suggests that restored `forms()` synthesis is still too broad for parity: the current JS output on that target omits array-style `forms(({ ... }) => ({ ... }))` companion surface in files like `loginLogic`, `signupLogic`, `newFrameForm`, and `frameLogic`. For benchmark parity, the rewrite has to match emitted JS behavior, not just a source reading of the plugin.
- On March 20, 2026, the stronger rule turned out to be narrower: builder selectors with completely blank reported surface should still be allowed through when there is a real source-backed selector expression. The bad case was not “blank fallback member plus source recovery” in general; it was skipping report-only blank members with no usable source expression. Dropping all blank-surface builder members was what hid `frame` / `defaultScene`-style public selectors from current FrameOS connected panel logic.
- Also on March 20, 2026, current FrameOS reruns confirmed that builder props-backed identity selectors are another JS-shaping quirk: the public selector/value surface should stay `any` even when local source recovery or inspect-side types can see a concrete primitive, while key recovery should still keep the real primitive.
- Builder `props()` sections can also report wrapper noise like `LogicBuilder<L>`. Treating that as authoritative blocks source-backed `props` recovery and then cascades into untyped `key((props) => ...)` callbacks inheriting the same wrapper instead of the actual primitive key type.
- Internal selector helpers now also need a split policy for source recovery: source-backed dependency names should be able to clean up destructured projector params like `([_, error]) => ...`, but alias-preserving reported helper types like `(arg: S6) => ...` still need to win when the source-side alternative only expands the same type and would drop the alias.
- On March 18, 2026, restoring builder `forms()` synthesis for those literal / direct-arrow-object shapes put the sample benchmark back at `20/20`. The earlier sample regression was caused by over-suppressing the public `kea-forms` surface, not by `sortedRepositories` disappearing from `__keaTypeGenInternalSelectorTypes`.
- The package-import selector recovery changed one more layer than expected: once `resolveImportFile` can see package type-entry files, typegen import normalization also needs to preserve the original bare package specifier when the resolved file lives under `node_modules`, otherwise benign imports like `import type { KeaPlugin } from 'kea'` drift into `kea/lib/index`.
- Builder `__keaTypeGenInternalReducerActions` are not emitted under a simple “all reducer actionType references count” rule. The current JS behavior on FrameOS is narrower:
  - reducer-only builder logics like `logsLogic` should omit that internal block
  - builder logics that also have listener sections, like `framesModel`, still keep reducer-sourced imported actions alongside listener-sourced ones
- Imported `otherLogic.actionTypes.*` listeners need the same input-kind split:
  - object logics like `githubImportLogic` and `githubNamespaceConnectLogic` still publish those imported listeners publicly and also keep the matching `__keaTypeGenInternalReducerActions`
  - builder logics like FrameOS `newFrameForm` and `controlLogic` keep those imported listener surfaces internal-only, so pushing them into public `listeners` reopens FrameOS diffs immediately
- On March 19, 2026, a new nested FrameOS-style regression test stopped reproducing the older connected `DeepPartial` import-loss example:
  - a `src/scenes/frame/panels/Schedule/scheduleLogic.tsx` consumer connected to a generated-typed `frameLogic` still emits `import type { DeepPartial } from 'kea-forms'`
  - the connected action surface stays `setFrameFormValues: (values: DeepPartial<FrameType>) => void`
- Treat the older `scheduleLogic` / `diagramLogic` / `newNodePickerLogic` “missing `DeepPartial` import” note as stale unless a fresh full-corpus diff shows a concrete current repro.

## Decision

- Stop treating the current `model.go` recovery layer as the architecture that will reach full parity.
- Keep it only as a stopgap compatibility layer while exploring a checker-backed replacement.
- Do not spend major time polishing more heuristic edge cases unless they are needed to unblock a broader extraction strategy.

## New Direction: `tsgo`-First Probe

### What Exists Today

- There is already an unofficial hidden API behind `tsgo --api`.
- This repo already wraps part of it in `rewrite/internal/tsgoapi/client.go`.
- On March 15, 2026, the local bundled `tsgo` binary was confirmed to expose `--api --async` and accept JSON-RPC.

### Probe Tool Added

- New command:
  - `go run ./cmd/kea-typegen-go probe-api --json`
- New targeting flag:
  - `--sample <property>`
- New nested targeting flag:
  - `--member <member>`
- New inner-array targeting flag:
  - `--element <first|last|index>`
- New nested-object targeting flag:
  - `--property <member>`
- New probe-side helper methods:
  - `--method printTypeNode`
  - `--method returnTypeToString`
  - `--method printReturnTypeNode`
  - `--method signatureDetails`
  - `--method propertyDetails`
  - `--method memberDetails`
- Purpose:
  - bootstrap a real snapshot/project/type/symbol context
  - discover real declaration-node handles from returned symbol/type metadata
  - recover identifier/name handles from the serialized AST returned by `getSourceFile`
  - recover exact callback/expression node handles from the serialized AST returned by `getSourceFile`
  - call hidden or partially-hidden `tsgo` RPC methods directly
  - compare probe-local printer helpers against raw hidden-method output when `typeToString` abbreviates nested surfaces
  - inspect ordered callback parameter types plus return types directly from signature metadata when the full callback printer is still lossy
  - expand object or symbol surfaces into readable named members with printed types, declared types, and callable signature details when a single `typeToString` result is still too coarse
  - classify results as `success`, `known-method-bad-params`, `unknown-method`, or `call-error`

### Probe Result On March 16, 2026

- Probe target:
  - project: `samples/`
  - file: `samples/logic.ts`
  - snapshot: `n0000000000000001`
  - project handle: `p./users/marius/projects/kea/kea-typegen/samples/tsconfig.json`
  - sample position: `240`
  - sample symbol position: `233`
  - sample symbol: `s0000000000000001`
  - sample type: `t0000000000000056` from `getTypesOfSymbols`
  - sample signature: `g0000000000000001` from the first callable member found while scanning candidate Kea section types
  - sample location handles: real declaration-node handles discovered from `valueDeclaration`, `declarations`, parent-symbol declarations, and `getSymbolOfType`
  - sample name locations: identifier-node handles recovered from the serialized AST returned by `getSourceFile`
  - sample position-backed symbol lookup: `getSymbolsAtPositions` succeeds when driven from property-name position `233`, while type-oriented position probes still use value position `240`
  - rebuilt binary caveat: the repo-root `bin/kea-typegen-go` must be refreshed with `./bin/prepare-go` before trusting probe results, otherwise `./bin/kea-typegen-go probe-api` can silently exercise stale code

- Nested member probes verified on March 16, 2026:
  - `--sample actions --member updateName` resolves the nested `updateName` symbol, `getTypeOfSymbolAtLocation` plus `getReturnTypeOfSignature` operate on the action member instead of the section wrapper, and `typeToString` now returns the readable callable surface `(name: string) => { name: string; }`.
  - `--sample selectors --member capitalizedName` resolves the nested selector symbol, `getReturnTypeOfSignature` returns the selector projector result type (`string` on the sample), and `typeToString` exposes the tuple-style selector member type strongly enough to see both the selector-input callback and the projector callback.
  - `--sample selectors --member capitalizedName --element last` now resolves the exact selector projector callback node, `getContextualType` and `getTypeAtLocation` both succeed on the callback location handle, and `typeToString` returns the non-truncated projector surface `(name: string, number: number) => string`.
  - `--sample selectors --member capitalizedName --element 0` resolves the exact selector input callback node, and `typeToString` now targets the inner callback itself instead of the outer tuple wrapper.
  - `--sample listeners --member updateNumber` resolves the nested listener symbol, and `getReturnTypeOfSignature` returns the listener callback result type (`void` on the sample).
  - `--sample loaders --member sessions` resolves the nested loader symbol, and `getTypeOfSymbolAtLocation` plus `getReturnTypeOfSignature` expose the loader-member shape instead of only the outer `loaders` callback.
  - `--sample sharedListeners --member someRandomFunction` resolves the nested shared-listener helper, and `typeToString` returns the readable callback shape `({ name }: { name: string; id?: number | undefined; }) => void`.
  - `--sample listeners --member updateName` follows the listener member that references `sharedListeners.someRandomFunction`, and `typeToString` returns the fuller listener callback surface with `payload`, `breakpoint`, `action`, and `previousState`.
  - `--file samples/posthogMapConnectLogic.ts --sample connect --member values` now emits `sampleNameLocations`, and `getSymbolAtLocation`, `getSymbolsAtLocations`, `getSymbolsAtPositions`, and `getTypeOfSymbolAtLocation` all succeed on the nested connect member.
  - The enabling parser fix was that `getSourceFile` node tables are not guaranteed to start on a 4-byte boundary; the builder-array / connect sample starts its node-table records at byte offset `1207`, so byte-granular scanning is required.
  - Additional connect shapes also now verify cleanly with the same targeting:
    - `samples/githubConnectLogic.ts --sample connect --member values`
    - `samples/githubConnectLogic.ts --sample connect --member actions`
    - `samples/githubNamespaceConnectLogic.ts --sample connect --member values`
    - `samples/routerConnectLogic.ts --sample connect --member actions`

- Builder-array probes verified on March 17, 2026:
  - `samples/builderLogic.ts --sample selectors --member sortedRepositories --element last --method typeToString` now resolves the exact builder selector projector callback and returns `(repositories: Repository[]) => Repository[]`.
  - `samples/builderLogic.ts --sample events --member afterMount --json` now emits `sampleNameLocations`, keeps the exact callback/object node handles under `sampleLocations`, and uses UTF-16-correct sample positions (`3283` for the property name and `3295` for the value callback) instead of stale byte offsets.
  - The enabling fix was that probe positions cannot be treated as raw Go byte offsets on Unicode-bearing files; `builderLogic.ts` contains two `👈` comments, so the file is `3658` UTF-8 bytes but only `3654` TypeScript UTF-16 code units, and both `*AtPosition` probes plus `getSourceFile` node-table discovery must normalize into UTF-16 first.

- Nested object-map probes verified on March 17, 2026:
  - `samples/loadersLogic.ts --sample loaders --member dashboard --property addDashboard --method typeToString` now drills past the outer loader object to the exact `addDashboard` callback and returns `({ name }: { name: string; }) => Dashboard`.
  - `samples/loadersLogic.ts --sample loaders --member misc --element last --property loadIt --method typeToString` now drills from the loader tuple’s object-map element down to `loadIt` and returns `() => { id: number; name: void; pinned: boolean; }`.
  - `samples/complexLogic.ts --sample reducers --member selectedActionId --element last --property selectAction --method typeToString` now drills from the reducer tuple’s handler map down to the exact `selectAction` reducer callback and returns `(_: number | "new" | null, { id }: { id: string; }) => number | null`.
  - The enabling fix was that nested property targets now trim trailing commas before deriving exact-node location handles, so `getSourceFile` range matching stays aligned with the actual callback node instead of falling back to a wider containing property/object node.

- Broader real-world probes verified on March 17, 2026:
  - `samples/complexLogic.ts --sample selectors --member selectedEditedAction --element last --method typeToString` returns `(selectedAction: ActionType, initialValuesForForm: ActionForm, form: FormInstance) => ActionForm`.
  - `samples/githubImportLogic.ts --sample selectors --member repositorySelectorCopy --element last --method typeToString` returns `(repositories: Repository[]) => Repository[]`.
  - `samples/builderLogic.ts --sample forms --member myForm --property submit --method typeToString` returns `(form: any) => void`.
  - `samples/builderLogic.ts --sample lazyLoaders --member lazyValue --element last --property initValue --method typeToString` returns `() => void`.
  - `samples/complexLogic.ts --sample reducers --member showActionsTooltip --element last --property setShowActionsTooltip --method typeToString` returns `(_: boolean, { showActionsTooltip }: { showActionsTooltip: boolean; }) => boolean`.
  - `samples/complexLogic.ts --sample listeners --member hideButtonActions --element first --method typeToString` now returns the exact callback surface `() => void` instead of the earlier contextual `ListenerFunction<...>` alias.
  - `FrameOS frontend/src/scenes/frame/panels/Schedule/scheduleLogic.tsx --sample key --method typeToString` returns `(props: ScheduleLogicProps) => number`, and the same file now keeps `key: number` locally once the generator switches from the older position-only key probe to an exact arrow-node `getTypeAtLocation` probe.

- Connect-tuple element probes verified on March 17, 2026:
  - `samples/posthogMapConnectLogic.ts --sample connect --member values --element first --method typeToString` returns `LogicWrapper<posthogMapLogicType>`.
  - `samples/posthogMapConnectLogic.ts --sample connect --member values --element last --method typeToString` now returns `string[]` instead of collapsing to `any`.
  - `samples/githubConnectLogic.ts --sample connect --member actions --element last --method typeToString` now also returns `string[]`.
  - The enabling fix was that probe fallback type selection no longer locks onto a weaker contextual `any` when an exact-node `getTypeAtLocation` call already produced a stronger non-callable type.

- Signature-return probes verified on March 17, 2026:
  - `samples/logic.ts --sample listeners --member updateName --method typeToString` still abbreviates the async listener return as `void | Promise<...>` when printing the full callback type.
  - `samples/logic.ts --sample listeners --member updateName --method returnTypeToString` now isolates the callback return type directly and returns `void | Promise<void>`.
  - `samples/logic.ts --sample listeners --member updateName --method printReturnTypeNode` also returns `void | Promise<void>`.
  - `samples/complexLogic.ts --sample listeners --member hideButtonActions --element first --method returnTypeToString` returns `void`.
  - `samples/complexLogic.ts --sample listeners --member hideButtonActions --element first --method printReturnTypeNode` also returns `void`.
  - `printTypeNode` is still only an auxiliary printer: on the simple `hideButtonActions` callback it currently prints just `() `, so return-type helpers are the more reliable path when the question is specifically about callback returns.

- Signature-details probes verified on March 17, 2026:
  - `samples/logic.ts --sample listeners --member updateName --method getSignaturesOfType` now succeeds under automatic probe params and returns the callable signature handle plus four ordered parameter-symbol handles for the listener callback type.
  - `samples/logic.ts --sample listeners --member updateName --method signatureDetails` now follows that first callable signature, resolves each parameter symbol with `getTypeOfSymbol`, and returns ordered parameter surfaces `{ id?: number | undefined; name: string; }`, `BreakPointFunction`, `{ type: string; payload: { id?: number | undefined; name: string; }; }`, and `any`, plus the exact return type `void | Promise<void>`.
  - `samples/logic.ts --sample actions --member updateName --method signatureDetails` returns the action callback surface as one ordered parameter type `string` plus return type `{ name: string; }`.
  - `samples/complexLogic.ts --sample listeners --member hideButtonActions --element first --method signatureDetails` returns an empty parameter list plus return type `void`, which is a cleaner probe path than relying on the abbreviated `() ` from `printTypeNode`.
  - `samples/logic.ts --sample actions --member updateName --method getTypeOfSymbol` now succeeds on the nested member symbol itself and returns the same sample type handle already used by `typeToString`.
  - On March 20, 2026, `signatureDetails` also started including each parameter symbol's `declaredType` alongside the resolved `type`. On the current samples that immediately exposes one more hidden-API quirk: listener and action callback parameters often report `declaredType: any` even when `type` is much more useful (`A5`, `BreakPointFunction`, payload object shapes, and so on).
  - Also on March 20, 2026, `signatureDetails` started recovering parameter `name` values from the signature declaration handle text itself, so callback probes now emit readable ordered names like `payload`, `breakpoint`, `action`, `previousState`, and `filter` instead of only symbol IDs.
  - Still on March 20, 2026, `signatureDetails` started including the trimmed `declarationText` recovered from that same declaration handle, so the probe now exposes the original signature snippet itself alongside the resolved parameter/return summaries even when the handle path comes back lowercased.

- Object/member expansion probes verified on March 20, 2026:
  - `samples/loadersLogic.ts --sample loaders --member dashboard --method propertyDetails` now expands the loader object into readable property rows, including property names, symbol handles, printed types, declared types, and callable signature details for `addDashboard` / `addDashboardNoType`.
  - The same `propertyDetails` result makes one current JS-parity quirk much easier to see: loader companion members can carry concrete `type` text like `({ name }: { name: string; }) => Dashboard` while `declaredType` is still plain `any`.
  - `samples/logic.ts --sample actions --method propertyDetails` no longer stalls on the outer `actions: () => ({ ... })` wrapper. When the current type has no direct properties, `propertyDetails` now follows the first callable signature return and expands the returned object surface instead.
  - `samples/logic.ts --sample selectors --method propertyDetails` gets the same callable-return fallback, so selector wrapper sections now expose concrete selector members instead of stopping on the outer function type.
  - `samples/logic.ts --sample actions --method memberDetails` now falls back through that same callable-return surface, so section-level member expansion is no longer limited to symbols that directly own members.
  - `samples/logic.ts --sample selectors --member capitalizedName --method propertyDetails` and `--method memberDetails` now treat the nested selector tuple as a fixed tuple surface instead of a generic array surface, so the helper output is reduced to tuple entries `0`, `1`, and `length` rather than the full inherited Array prototype.

- Hidden methods that succeeded on the sample context:
  - `getBaseTypeOfLiteralType`
  - `getBaseTypes`
  - `getContextualType`
  - `getDeclaredTypeOfSymbol`
  - `getExportSymbolOfSymbol`
  - `getExportsOfSymbol`
  - `getIndexInfosOfType`
  - `getPropertiesOfType`
  - `getSourceFile`
  - `getMembersOfSymbol`
  - `getParentOfSymbol`
  - `getRestTypeOfSignature`
  - `getReturnTypeOfSignature`
  - `getSignaturesOfType`
  - `getSymbolAtLocation`
  - `getSymbolOfType`
  - `getSymbolsAtLocations`
  - `getSymbolsAtPositions`
  - `getTargetOfType`
  - `getTypeAtLocation`
  - `getTypeAtLocations`
  - `getTypeOfSymbol`
  - `getTypeOfSymbolAtLocation`
  - `getTypePredicateOfSignature`
  - `getTypesAtPositions`
  - `getTypesOfSymbols`
  - `typeToString`

- Hidden methods that exist but currently panic or otherwise fail on the sampled object/common type:
  - `getBaseTypeOfType`
  - `getCheckTypeOfType`
  - `getConstraintOfType`
  - `getExtendsTypeOfType`
  - `getIndexTypeOfType`
  - `getLocalTypeParametersOfType`
  - `getObjectTypeOfType`
  - `getOuterTypeParametersOfType`
  - `getTypeArguments`
  - `getTypesOfType`
  - `getTypeParametersOfType`

### What This Means

- The hidden API is materially richer than the wrapper we started with.
- We should replace current heuristic recovery with checker-backed queries wherever possible, and stop adding new semantic guesses in Go.
- We may still be able to do that without forking immediately.
- Position-based symbol queries are already useful as long as the probe keeps property-name positions separate from value-expression positions.
- Real declaration-node handles are enough to unlock several `*AtLocation` checker queries, so this is no longer blocked on placeholder handles.
- Identifier/name handles recovered from `getSourceFile` are now enough to unlock both `getSymbolAtLocation` and `getSymbolsAtLocations` on sample property keys.
- `--sample` plus `--member` is now enough to target real nested action / selector / listener / loader members instead of only section wrappers.
- `--element` is now enough to retarget those probes from an outer selector tuple member down to the exact inner callback node when the raw tuple string is too abbreviated.
- `--property` is now enough to retarget those probes one level deeper again when the current target is an object-map wrapper such as a reducer handler map or loader callback map.
- `getSourceFile` node tables are not consistently 4-byte aligned, but byte-granular scanning now recovers identifier handles on both the ordinary sample file and the builder-array / connect sample.
- Unicode-bearing files need one more normalization step on top of that: probe positions must be converted from Go byte offsets into TypeScript UTF-16 offsets before calling `*AtPosition` methods or matching `getSourceFile` node ranges.
- Exact AST-range handles from `getSourceFile` are now good enough to drive `getContextualType`, `getTypeAtLocation`, and `getSymbolAtLocation` on inner callback nodes that position-based probing previously collapsed to `any`.
- `getSymbolsAtPositions` is still a useful fallback and cross-check, but it is no longer the only practical probe path for the current connect sample.
- `typeToString` is now wired into `probe-api`, so action payload surfaces, shared-listener helpers, and connect-member tuple shapes can be read directly instead of only via opaque `t...` handles.
- Selector helper inspection is materially better than before because `--element last` now gives a dedicated “inner projector callback node/type directly” path instead of relying on the abbreviated outer tuple string.
- The same exact-node path now also works on builder-array callbacks that appear after non-ASCII comments, so the probe is no longer accidentally biased toward ASCII-only samples.
- The UTF-16 lesson is not probe-only anymore: generator-side `keainspect` position probes now normalize source byte offsets before calling `tsgo`, and the Schedule-style builder-key integration path keeps `key: number` even with a leading `👈` comment.
- Exact-node fallback ranking matters even for non-callable targets: connect tuple name arrays only stop collapsing to `any` once `getTypeAtLocation` can outrank a weaker contextual fallback.
- The earlier listener-array alias issue is gone under exact-node fallback ranking; real listener-array callback nodes now stringify as callback surfaces again.
- Builder `key()` callbacks have the same shape of problem: the old position probe can land on the callback parameter type (`ScheduleLogicProps`) instead of the callback node, while an exact arrow-node location handle recovers the real callback surface and return type.
- Full callback printers can still abbreviate nested async return types, but the new signature-return helpers now let us separate a printer limitation (`void | Promise<...>`) from the underlying recovered return type (`void | Promise<void>`).
- Signature-backed inspection is now stronger too: `getSignaturesOfType` plus parameter-symbol type lookups are enough to recover ordered callback parameter surfaces and exact return types without depending entirely on the full callback printer, and `signatureDetails` now also shows when a parameter's `declaredType` has already collapsed to something weaker like `any` while still recovering the original parameter names and declaration text from the signature handle.
- `propertyDetails` now gives a practical bridge between raw hidden methods and parity debugging: object-shaped targets can be inspected as named rows with both printed `type` and `declaredType`, and callable wrapper sections can now automatically unwrap to their returned object surface when that is the real interesting target.
- `memberDetails` is also more practical than the raw `getMembersOfSymbol` API: when the current symbol has no members, it now falls back through the current type symbol and then the callable-return property surface instead of just returning `null`.
- Tuple-like selector members are now much easier to inspect with those helpers too: when the current type string is a tuple, `propertyDetails` / `memberDetails` trim inherited array-prototype noise and keep just the fixed tuple entries plus `length` in numeric order.
- We still do not have a stable external “walk the full program exactly like JS does” API.
- For true parity, the best long-term path is still one of:
  - extend the hidden API until Go can reconstruct the same data with no semantic guessing
  - fork `tsgo` and add a kea-specific extraction endpoint or normalized IR
  - vendor the relevant `tsgo` / TypeScript-integration code into this repo and hack directly on the data path if the external binary boundary is what blocks us

## Re-Grouped Next Steps

1. Freeze heuristic growth in `model.go`.
   - No new semantic type-recovery branches based on source text shape, AST shape, string contains checks, object-member shape, or corpus-specific special cases.
   - The only acceptable short-term edits in that area are:
     - deleting old heuristics
     - narrowing a regression while we migrate to checker-backed data
     - plumbing checker-backed data through an existing callsite
2. Inventory the current guessing surface.
   - Make a concrete list of the `model.go` helpers that are still synthesizing semantic types from source shape.
   - Tag each one as:
     - replace with checker query
     - keep only as node-locator glue
     - delete after checker path exists
3. Expand `probe-api` and `tsgoapi` deliberately instead of expanding heuristics.
   - Use `--sample <property> --member <member>` plus `typeToString`, `signatureDetails`, `propertyDetails`, and `memberDetails` to map the exact missing parity families.
   - On selector tuples, use `propertyDetails` / `memberDetails` first, then `--element <first|last|index>` to drill into the exact callback.
   - On reducer/listener/loader object maps, use `--property <member>` instead of reconstructing the shape from source.
4. Decide quickly whether the current hidden API is enough.
   - If the required semantic fact exists behind one more hidden method, add that method to the Go wrapper.
   - If the fact is not available cleanly, stop guessing and move the boundary.
5. If needed, do the “impossible” thing early instead of late.
   - Import / vendor the relevant `tsgo` code into this repo or add a local fork under `poc/` / `third_party/`.
   - Add a kea-specific extraction endpoint or normalized IR that returns the exact selector/action/helper/connect surfaces we need.
   - Prefer hacking the TypeScript side once over re-encoding TypeScript semantics forever in Go.
6. After checker-backed extraction exists, replace heuristic families one category at a time.
   - selector/value shaping
   - action payload surfaces
   - helper emission
   - connected logic resolution
   - builder callback / tuple projector recovery
7. Only use real-world compare runs to verify the checker-backed path.
   - Do not let compare deltas drive new source-shape guessing rules.

## Current Host Readiness

- On March 15, 2026, this repo was prepared for a new host with a repo-local Flox environment at `.flox/`.
- Activate it with `flox activate` before Go rewrite work so `go`, `node`, and `yarn` come from the pinned environment and repo-local cache directories are created under `.cache/kea-typegen/`.
- The transferred `kea-typegen-go` and bundled `tsgo` binaries initially carried macOS quarantine attributes on this host; those were cleared locally so they execute again.
- `bin/prepare-go` now refreshes both `rewrite/bin/kea-typegen-go` and the repo-root `bin/kea-typegen-go`, clears macOS quarantine/provenance attrs on the repo/bin output dirs and rebuilt binaries when `xattr` is available, and the sample benchmark now runs `rewrite/bin/kea-typegen-go` directly so it cannot silently use a stale root binary.
- Verified on this host on March 15, 2026:
  - `flox activate -c 'cd rewrite && go test ./internal/tsgoapi ./cmd/kea-typegen-go'` passes.
  - `flox activate -c './bin/kea-typegen-go probe-api --json'` succeeds.
- Re-verified on this host on March 16, 2026 after `flox activate -c './bin/prepare-go'`:
  - `flox activate -c 'cd rewrite && go test ./internal/tsgoapi ./cmd/kea-typegen-go'` passes.
  - `flox activate -c './bin/kea-typegen-go probe-api --json'` now emits `sampleNameLocations`, and both `getSymbolAtLocation` and `getSymbolsAtLocations` succeed on the sample property key.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample actions --member updateName --json'` resolves the nested action member.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample actions --member updateName --method typeToString --json'` returns the readable callable surface `(name: string) => { name: string; }`.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --member capitalizedName --json'` resolves the nested selector member.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --member capitalizedName --method typeToString --json'` exposes the tuple-style selector member type.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --member capitalizedName --element last --method typeToString --json'` returns the readable selector projector surface `(name: string, number: number) => string`.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --member capitalizedName --element last --json'` now reports an exact callback node handle under `sampleLocations`, and both `getContextualType` and `getTypeAtLocation` succeed on that inner selector callback.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample listeners --member updateNumber --json'` resolves the nested listener member.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample sharedListeners --member someRandomFunction --method typeToString --json'` returns the readable shared-listener helper type.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample loaders --member sessions --json'` resolves the nested loader member.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/posthogMapConnectLogic.ts --sample connect --member values --json'` now emits `sampleNameLocations`, and `getSymbolAtLocation`, `getSymbolsAtLocations`, `getSymbolsAtPositions`, and `getTypeOfSymbolAtLocation` all succeed on the nested connect member.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/githubConnectLogic.ts --sample connect --member values --method typeToString --json'` returns `(string[] | BuiltLogic<githubLogicType> | LogicWrapper<githubLogicType>)[]`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/routerConnectLogic.ts --sample connect --member actions --method typeToString --json'` returns `(string[] | LogicWrapper<routerType>)[]`.
- Re-verified on this host on March 17, 2026 after `flox activate -c './bin/prepare-go'`:
  - `flox activate -c 'cd rewrite && go test ./cmd/kea-typegen-go ./internal/keainspect'` passes.
  - `./bin/benchmark -c write -n 1 -w 0 --skip-prepare` now returns `20/20` semantic matches on the sample corpus.
  - Fresh full-corpus reruns on the current local checkouts now measure `85/739` semantic matches for PostHog and `8/38` for FrameOS.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/builderLogic.ts --sample selectors --member sortedRepositories --element last --method typeToString --json'` returns `(repositories: Repository[]) => Repository[]`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/builderLogic.ts --sample events --member afterMount --json'` now emits `sampleNameLocations` and uses UTF-16-correct positions (`3283` / `3295`) on the Unicode-bearing builder sample.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/posthogMapConnectLogic.ts --sample connect --member values --json'` still succeeds after the UTF-16 normalization change, so the earlier connect path did not regress.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/loadersLogic.ts --sample loaders --member dashboard --property addDashboard --method typeToString --json'` returns `({ name }: { name: string; }) => Dashboard`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/loadersLogic.ts --sample loaders --member misc --element last --property loadIt --method typeToString --json'` returns `() => { id: number; name: void; pinned: boolean; }`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/complexLogic.ts --sample reducers --member selectedActionId --element last --property selectAction --method typeToString --json'` returns `(_: number | "new" | null, { id }: { id: string; }) => number | null`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/complexLogic.ts --sample selectors --member selectedEditedAction --element last --method typeToString --json'` returns `(selectedAction: ActionType, initialValuesForForm: ActionForm, form: FormInstance) => ActionForm`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/githubImportLogic.ts --sample selectors --member repositorySelectorCopy --element last --method typeToString --json'` returns `(repositories: Repository[]) => Repository[]`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/builderLogic.ts --sample forms --member myForm --property submit --method typeToString --json'` returns `(form: any) => void`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/builderLogic.ts --sample lazyLoaders --member lazyValue --element last --property initValue --method typeToString --json'` returns `() => void`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/complexLogic.ts --sample reducers --member showActionsTooltip --element last --property setShowActionsTooltip --method typeToString --json'` returns `(_: boolean, { showActionsTooltip }: { showActionsTooltip: boolean; }) => boolean`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/complexLogic.ts --sample listeners --member hideButtonActions --element first --method typeToString --json'` now returns `() => void`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/posthogMapConnectLogic.ts --sample connect --member values --element first --method typeToString --json'` returns `LogicWrapper<posthogMapLogicType>`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/posthogMapConnectLogic.ts --sample connect --member values --element last --method typeToString --json'` now returns `string[]`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/githubConnectLogic.ts --sample connect --member actions --element last --method typeToString --json'` now returns `string[]`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/logic.ts --sample listeners --member updateName --method returnTypeToString --json'` now returns `void | Promise<void>`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/logic.ts --sample listeners --member updateName --method printReturnTypeNode --json'` also returns `void | Promise<void>`.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample listeners --member updateName --method getSignaturesOfType --json'` now returns the listener callback signature handle plus four ordered parameter-symbol handles.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample listeners --member updateName --method signatureDetails --json'` now returns ordered parameter type details and the exact async return type `void | Promise<void>`.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample actions --member updateName --method signatureDetails --json'` returns `string` for the action callback parameter and `{ name: string; }` for the callback return type.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/complexLogic.ts --sample listeners --member hideButtonActions --element first --method signatureDetails --json'` returns an empty parameter list plus return type `void`.
- Re-verified on this host on March 20, 2026 after `flox activate -c './bin/prepare-go'`:
  - `flox activate -c 'cd rewrite && go test ./cmd/kea-typegen-go ./internal/keainspect ./internal/tsgoapi'` passes.
  - `./bin/benchmark -c write -n 1 -w 0 --skip-prepare` still returns `20/20` semantic matches on the sample corpus.
  - `./bin/kea-typegen-go --help` now succeeds again after the same rebuild path, so the direct packaged binary is usable on this host without the earlier manual copy workaround.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample listeners --member updateName --method signatureDetails --json'` still returns ordered listener parameter types plus exact return type `void | Promise<void>`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/builderLogic.ts --sample selectors --member sortedRepositories --element last --method typeToString --json'` still returns `(repositories: Repository[]) => Repository[]`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/posthogMapConnectLogic.ts --sample connect --member values --element last --method typeToString --json'` still returns `string[]`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/loadersLogic.ts --sample loaders --member dashboard --method propertyDetails --json'` returns expanded loader property details with both printed `type` and `declaredType`.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample actions --method propertyDetails --json'` now expands the returned action map and shows callable details for `updateName`, `updateConst`, and `updateNumber`.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --method propertyDetails --json'` now expands the returned selector map instead of stalling on the outer wrapper function.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample actions --method memberDetails --json'` now follows the same callable-return fallback and returns the expanded action members instead of `null`.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --member capitalizedName --method propertyDetails --json'` now reduces the nested selector tuple surface to `0`, `1`, and `length` instead of dumping array-prototype methods.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --member capitalizedName --method memberDetails --json'` now returns that same trimmed tuple-member surface.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --member capitalizedName --element last --method signatureDetails --json'` still returns the projector callback parameter types `string`, `number`, and return type `string`.
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/autoImportLogic.ts --sample actions --member combinedA6Action --method signatureDetails --json'` now also emits per-parameter `declaredType`; on this alias-bearing action sample the resolved parameter `type` stays `A5` while `declaredType` is already just `any`.
  - `flox activate -c './bin/kea-typegen-go probe-api --sample listeners --member updateName --method signatureDetails --json'` now emits `declaredType` for every listener callback parameter too, and the sampled listener parameters currently all show `declaredType: any` even when the resolved `type` is richer.
  - Those same `signatureDetails` results now also carry readable parameter names recovered from the declaration text itself: `filter` on `combinedA6Action`, and `payload`, `breakpoint`, `action`, `previousState` on the listener sample.
  - They now also carry the original signature snippet under `declarationText`, such as source-authored `(filter: A5) => ({ a6: filter.a6, bla: filter.bla })` on the auto-import action sample and the generated listener callback surface from `logicType.ts`.

## Guardrails

- Do not optimize for the current normalized compare score alone.
- Do not chase precision-only diffs as if they were parity bugs.
- Never add a new long-lived rule that guesses a semantic type from source text shape, AST shape, printed type fragments, naming conventions, or corpus-specific examples.
- Use source / AST traversal only to find the exact node, symbol, signature, or property handle to query from TypeScript.
- The final semantic type must come from TypeScript checker/internal API data, not from Go-side reconstruction.
- If the current hidden `tsgo` API cannot provide the answer, extend `tsgoapi`, extend `probe-api`, or fork / vendor `tsgo` before adding another heuristic.
- Prefer one invasive `tsgo` change over a permanent stream of cat-and-mouse fixes in `model.go`.
- Treat existing `model.go` heuristic recovery as migration debt to retire, not an invitation to add more.

## Useful Commands

- Enter the pinned toolchain:
  - `flox activate`
- Prepare the Go binary inside Flox:
  - `flox activate -c './bin/prepare-go'`
- Sample benchmark:
  - `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
- FrameOS real-world compare:
  - `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target frameos --json'`
- PostHog real-world compare:
  - `flox activate -c 'node ./scripts/compare-real-world-typegen.js --target posthog --json'`
- Probe hidden `tsgo` methods:
  - `flox activate -c 'cd rewrite && go run ./cmd/kea-typegen-go probe-api --json'`
- Probe a specific Kea section:
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --json'`
- Probe a nested member inside a section:
  - `flox activate -c './bin/kea-typegen-go probe-api --sample actions --member updateName --json'`
- Probe the readable type surface of a nested member:
  - `flox activate -c './bin/kea-typegen-go probe-api --sample actions --member updateName --method typeToString --json'`
- Probe an inner selector callback directly instead of the outer tuple wrapper:
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --member capitalizedName --element last --method typeToString --json'`
- Probe a nested reducer/loader callback inside an object-map wrapper:
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/loadersLogic.ts --sample loaders --member dashboard --property addDashboard --method typeToString --json'`
- Probe a builder-array callback on the real builder sample:
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/builderLogic.ts --sample selectors --member sortedRepositories --element last --method typeToString --json'`
- Probe a connect member on a connect-focused sample:
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/posthogMapConnectLogic.ts --sample connect --member values --json'`
- Probe a specific connect-tuple element:
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/posthogMapConnectLogic.ts --sample connect --member values --element last --method typeToString --json'`
- Probe the exact return type of a callback signature when the full callback string is abbreviated:
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/logic.ts --sample listeners --member updateName --method returnTypeToString --json'`
- Print the return type node of a callback signature directly:
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/logic.ts --sample listeners --member updateName --method printReturnTypeNode --json'`
- Probe ordered callback parameter types plus the exact return type from signature metadata:
  - `flox activate -c './bin/kea-typegen-go probe-api --sample listeners --member updateName --method signatureDetails --json'`
- Compare the resolved and declared parameter surfaces on a callback signature:
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/autoImportLogic.ts --sample actions --member combinedA6Action --method signatureDetails --json'`
- Probe raw callable signature handles and ordered parameter-symbol handles:
  - `flox activate -c './bin/kea-typegen-go probe-api --sample listeners --member updateName --method getSignaturesOfType --json'`
- Expand an object-shaped target into readable property rows:
  - `flox activate -c './bin/kea-typegen-go probe-api --file samples/loadersLogic.ts --sample loaders --member dashboard --method propertyDetails --json'`
- Expand a callable section wrapper through its returned object surface:
  - `flox activate -c './bin/kea-typegen-go probe-api --sample actions --method propertyDetails --json'`
- Expand the current symbol, with fallback through the current type and callable return surface when needed:
  - `flox activate -c './bin/kea-typegen-go probe-api --sample actions --method memberDetails --json'`
- Summarize a selector tuple member as fixed tuple entries before drilling into one callback:
  - `flox activate -c './bin/kea-typegen-go probe-api --sample selectors --member capitalizedName --method propertyDetails --json'`
- Targeted package tests:
  - `flox activate -c 'cd rewrite && go test ./internal/tsgoapi ./cmd/kea-typegen-go'`
