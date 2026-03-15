# Go Rewrite TODO

## Goal

- Keep the repo-local sample benchmark at `./bin/benchmark -c write -n 1 -w 0 --skip-prepare` with `20/20` semantic matches.
- Improve real-world parity against the existing TypeScript generator.
- Treat semantic TypeScript AST parity as the current scorekeeping metric, but do not confuse that metric with actual type-correctness.

## Verified Status

### Samples

- Current rebuilt-host verification on March 15, 2026 with `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`.
- Result on the current checkout: `18/20` semantic matches, `90.0%`.
- Current sample semantic diffs: `builderLogicType.ts`, `complexLogicType.ts`.

### PostHog

- Latest verified saved full compare on March 15, 2026:
  - comparable files: `739`
  - semantic matches: `286`
  - semantic diffs: `453`
  - semantic accuracy: `38.70%`
- The March 15, 2026 saved clean summary lived under a host-local `/tmp` directory on the previous machine and should be regenerated on this host before reuse.
- Important caveat: the latest local `queryDatabase` action-payload nullish fix is unit-tested but was not folded into a fresh full PostHog rerun, so `38.70%` is still the latest verified corpus-wide number.

### FrameOS

- Latest verified full compare on March 15, 2026:
  - comparable files: `39`
  - semantic matches: `11`
  - semantic diffs: `28`
  - semantic accuracy: `28.21%`

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

- Samples are no longer representative of real-world parity. `20/20` samples can coexist with sub-40% PostHog and sub-30% FrameOS.
- Recent local fixes can still move individual files, but the overall pattern is whack-a-mole: fix one family, reopen another.
- The latest local action-payload fix stopped `setTreeRef`-style nullish drift, but it also made the deeper issue more obvious: many remaining diffs are about faithfully reproducing TS-generator shaping decisions, not just recovering missing obvious types.
- Precision-only differences are real and common. We should stop treating them as equivalent to parity bugs when making strategy decisions.

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
- Purpose:
  - bootstrap a real snapshot/project/type/symbol context
  - discover real declaration-node handles from returned symbol/type metadata
  - call hidden or partially-hidden `tsgo` RPC methods directly
  - classify results as `success`, `known-method-bad-params`, `unknown-method`, or `call-error`

### Probe Result On March 15, 2026

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
  - sample position-backed symbol lookup: `getSymbolsAtPositions` succeeds when driven from property-name position `233`, while type-oriented position probes still use value position `240`

- Hidden methods that succeeded on the sample context:
  - `getBaseTypeOfLiteralType`
  - `getBaseTypes`
  - `getContextualType`
  - `getDeclaredTypeOfSymbol`
  - `getExportSymbolOfSymbol`
  - `getExportsOfSymbol`
  - `getIndexInfosOfType`
  - `getSourceFile`
  - `getMembersOfSymbol`
  - `getParentOfSymbol`
  - `getRestTypeOfSignature`
  - `getReturnTypeOfSignature`
  - `getSymbolOfType`
  - `getSymbolsAtPositions`
  - `getTargetOfType`
  - `getTypeAtLocation`
  - `getTypeAtLocations`
  - `getTypeOfSymbolAtLocation`
  - `getTypePredicateOfSignature`
  - `getTypesAtPositions`
  - `getTypesOfSymbols`

- Hidden methods that are recognized and callable, but still need identifier-level handles instead of declaration-node handles:
  - `getSymbolAtLocation`
  - `getSymbolsAtLocations`

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
- We can likely replace some current heuristic recovery with checker-backed queries without forking immediately.
- Position-based symbol queries are already useful as long as the probe keeps property-name positions separate from value-expression positions.
- Real declaration-node handles are enough to unlock several `*AtLocation` checker queries, so this is no longer blocked on placeholder handles.
- Declaration-node handles are still not enough for `getSymbolAtLocation` or `getSymbolsAtLocations`, so identifier/name-handle discovery remains the main open blocker.
- `getSourceFile` now gives us a serialized AST blob, which is the strongest current lead for extracting real identifier/name handles without forking immediately.
- We still do not have a stable external “walk the full program exactly like JS does” API.
- For true parity, the best long-term path is still one of:
  - extend the hidden API until Go can reconstruct the same data with minimal guessing
  - fork `tsgo` and add a kea-specific extraction endpoint or normalized IR

## Recommended Next Steps

1. Expand the probe tool deliberately instead of expanding heuristics.
2. Push node-handle discovery one level deeper from declaration-node handles to identifier/name handles so `getSymbolAtLocation` and `getSymbolsAtLocations` stop returning `null`.
   - Start with the serialized AST returned by `getSourceFile`, since plain start/end/kind guessing has not been enough to recover working identifier handles.
3. Map hidden methods onto the exact missing parity families:
   - selector/value shaping
   - action payload surface
   - helper emission
   - connected logic resolution
4. Decide quickly whether the hidden API is enough.
5. If it is not enough, fork `tsgo` and add a kea-specific extraction API instead of continuing with source-recovery guesswork.

## Current Host Readiness

- On March 15, 2026, this repo was prepared for a new host with a repo-local Flox environment at `.flox/`.
- Activate it with `flox activate` before Go rewrite work so `go`, `node`, and `yarn` come from the pinned environment and repo-local cache directories are created under `.cache/kea-typegen/`.
- The transferred `kea-typegen-go` and bundled `tsgo` binaries initially carried macOS quarantine attributes on this host; those were cleared locally so they execute again.
- `bin/prepare-go` now refreshes both `rewrite/bin/kea-typegen-go` and the repo-root `bin/kea-typegen-go`, and the sample benchmark now runs `rewrite/bin/kea-typegen-go` directly so it cannot silently use a stale root binary.
- Verified on this host on March 15, 2026:
  - `flox activate -c 'cd rewrite && go test ./internal/tsgoapi ./cmd/kea-typegen-go'` passes.
  - `flox activate -c './bin/kea-typegen-go probe-api --json'` succeeds.

## Guardrails

- Do not optimize for the current normalized compare score alone.
- Do not chase precision-only diffs as if they were parity bugs.
- Do not keep growing `model.go` heuristics unless the added logic is clearly temporary or directly informs the new checker-backed path.

## Useful Commands

- Enter the pinned toolchain:
  - `flox activate`
- Prepare the Go binary inside Flox:
  - `flox activate -c './bin/prepare-go'`
- Sample benchmark:
  - `flox activate -c './bin/benchmark -c write -n 1 -w 0 --skip-prepare'`
- Probe hidden `tsgo` methods:
  - `flox activate -c 'cd rewrite && go run ./cmd/kea-typegen-go probe-api --json'`
- Targeted package tests:
  - `flox activate -c 'cd rewrite && go test ./internal/tsgoapi ./cmd/kea-typegen-go'`
