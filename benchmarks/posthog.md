# PostHog full-project typegen benchmark

This benchmark runs `kea-typegen check` from the PostHog repository root. Its
`.kearc` selects inline output and `tsconfig.kea-typegen.json` covers the frontend
and product directories. The checked-in generated types provide a common input
and a reference for detecting output changes.

## Results

Native TypeScript reduced median peak RSS by 1.46 GiB (18.4%) against PostHog's
installed typegen and increased median wall time by 4.09 s (6.8%). Against the
classic engine of the same typegen revision, native used 14.4% less memory and
took 5.1% longer. These measurements cover a one-shot full-project check.

| Engine | Wall time per run (s) | Median (s) | Peak RSS per run (MiB) | Median (GiB) | Files flagged |
| --- | --- | ---: | --- | ---: | ---: |
| PostHog, TS 6.0.3 | 60.494, 59.693, 60.489 | 60.489 | 8136, 8139, 8138 | 7.95 | 0 |
| Classic, TS 5.9.3 | 61.418, 61.524, 61.416 | 61.418 | 7670, 8021, 7755 | 7.57 | 188 |
| Native, TS 7.1.0-beta | 60.772, 64.888, 64.581 | 64.581 | 6593, 6785, 6642 | 6.49 | 283 |

All nine checks found 1,725 logics in 1,724 managed files. Each engine
flagged the same paths on all three runs. Native flagged every file that
classic flagged, plus 95 more. Two single-file writes in the temporary
checkout reproduced examples of each kind of difference.

- Both engines changed `interProjectCopyLogic` to use an `import type` for
  `OrganizationType` in place of an inline `import('../../types').OrganizationType`.
- Only native changed `taskCommentContextsLogic`. In two inferred action
  payloads it printed `screenshot?: string` where the checked-in type reads
  `screenshot?: string | undefined`.

## Where the time goes

In a separate full check of the checked-in files, timestamps around
`visitProgram` and `printToFiles` gave these wall times. Both engines used
the same typegen revision. Each column is one diagnostic run.

| Phase | Classic 5.9.3 | Native 7.1 beta |
| --- | ---: | ---: |
| CLI startup and project setup, up to the start of the visitor | 12.4 s | 1.2 s |
| Find logics, run visitors and query the checker | 43.1 s | 56.8 s |
| Print and compare with the checked-in files | 3.4 s | 3.4 s |

TypeScript's native API timing collector counted 319,624 synchronous requests.
They took 36.4 s in total. The Go server reported 23.7 s processing them,
leaving an estimated 12.7 s of transport overhead. These timings are included
in the phases above. The client received 400 MB of responses. The most common
request was `getSymbolAtLocation` (127,654 calls, 6.6 s round trip). The
largest total time was `getTypeAtLocation` (23,108 calls, 17.0 s including
inference). The adapter also parses source and inferred types in JavaScript
for the existing Kea visitors.

Set `TS_API_TIMING_DIR` to an output directory when running a native check to
get TypeScript's request count, round-trip time, server time, and byte totals.
The SDK also supports batched symbol and type lookups, which can reduce the
number of round trips from JavaScript. [TypeScript API batching](https://github.com/microsoft/TypeScript/issues/63903)

## Checks with nothing to write

A fresh full-project `check` visits the logics and regenerates the expected
types in memory before it can tell whether the files are current. Changes to
imported types can affect a logic whose own source has stayed the same.
`--use-cache` restores missing external type files during a write. It does
not cache the result of a check.

To measure this path, we first ran `write --compiler native` in a separate
copy of PostHog. It wrote 283, 95, 23, and finally zero files over four passes
in 246 s. Three fresh native checks of those outputs took 64.18, 65.31 and
61.77 s. Each returned success and checked all 1,725 logics. The 64.18 s median
is close to the 64.58 s median on the original checkout with 283 files flagged.

With a narrower scope, the same typegen revision produced the following median
times over three fresh processes per engine. Both engines found zero changes
on every run.

| PostHog scope | Managed logics | Classic 5.9.3 | Native 7.1 beta | Speedup |
| --- | ---: | ---: | ---: | ---: |
| One frontend feature flag logic | 1 | 10.20 s | 1.31 s | 7.8× |
| Replay shared package | 0 | 0.620 s | 0.414 s | 1.5× |

- `check --file frontend/src/lib/logic/featureFlagLogic.ts` generates and
  compares one logic using the project's compiler options.
- `check --config common/replay-shared/tsconfig.json` checks a workspace
  package that contains zero Kea logics.

## Method

The workload is [PostHog at bfd9dbd](https://github.com/PostHog/posthog/commit/bfd9dbd1278ca51e105d3be60f34aed4716a7679).
The baseline is its locked kea-typegen 3.8.3 with TypeScript 6.0.3. The other
two alternatives use the same typegen revision, with either `--compiler classic`
or `--compiler native`. All three use Node 26.13.1, PostHog's 16 GiB V8 heap
limit, and the default CLI formatting behavior. [Typegen revision](https://github.com/keajs/kea-typegen/commit/98dc82845c404c40d4e6d91d5dbc998eb8df0874)

The runner starts a fresh CLI process for each check, runs the engines
sequentially, and rotates their order across three rounds. Installation and
building are outside the timer. Each check shares the OS file cache. Wall time
includes startup, type inference, printing and comparison with disk. Peak
RSS is the largest sum of resident memory sampled across the process tree at
roughly 100 ms intervals. This includes the Go compiler child and can count
shared pages in both processes. The host ran Linux on an AMD EPYC 9454P with
96 logical CPUs and 251 GiB RAM.

## Reproduce

From the kea-typegen checkout, with its locked dependencies installed, use
Node 26.13.1 and pnpm 11.28.5. Run these commands to prepare the measured PostHog revision.

```sh
work=$(mktemp -d)
git clone --quiet --local --shared ../../PostHog/posthog "$work/posthog"
git -C "$work/posthog" checkout --quiet --detach bfd9dbd1278ca51e105d3be60f34aed4716a7679
(
    cd "$work/posthog" &&
    pnpm --filter @posthog/root --filter @posthog/frontend... install --frozen-lockfile --ignore-scripts
)
npm run build
python3 benchmarks/posthog.py "$work/posthog" "$work/results"
```

The runner writes one verbose CLI log per run and the elapsed time, sampled
peak RSS, exit status, and output-change count to `results.json`. A completed
check exits 1 when it finds files to regenerate. A failed engine is recorded
and skipped in subsequent rounds. Use `--runs 1` for one pass per engine.

## Go typegen

A full PostHog check with the Go engine took **29.07 s** at the median, down
from **64.49 s** with kea-typegen 3.9.0's native engine. Both found 1,725
logics on every run. Go generated 1,557 declarations directly. JavaScript
plugin compatibility handled the remaining 168.

| Engine | Full check runs (s) | Median | Median peak process-tree RSS | Inline files flagged |
| --- | --- | ---: | ---: | ---: |
| 3.9.0, `--compiler native` | 66.38, 57.74, 64.49 | 64.49 s | 6.44 GiB | 283 |
| Go port, `--compiler go` | 29.64, 28.89, 29.07 | 29.07 s | 5.24 GiB | 284 |

The Go check was 2.22× faster and used 18.6% less peak memory by these
medians. The runs used the same PostHog commit, installed dependencies, Node
version, heap limit and Linux host described above. Each started a fresh CLI
process on the original checked-in types. The timer includes plugin execution,
formatting and comparison with disk. A separate check with the final build
took 29.14 s with 284 files flagged. The native baseline was built from the
[3.9.0 release revision](https://github.com/keajs/kea-typegen/commit/147ea35a510e0611c06e8cd7e5c7137eb32d6046).

Ignoring whitespace and comments, 1,721 of the 1,725 generated inline
declarations had identical syntax trees. The four differences were in
`annotationsModel`, `playerMetaLogic`, `logsImpactLogic`, and
`chartAlternativesLogic`. Go preserved indexed-access types in the first three
and a string literal in the last. In a separate copy, a full `write --compiler go`
updated 284, 98, 22, then zero files over successive passes as dependent
logics picked up the new types. A fresh full check then exited successfully.

To reproduce the paired comparison, prepare `$work/posthog` as above, then run
the following from the kea-typegen checkout with Go 1.27 or newer available.

```sh
mkdir "$work/release"
git archive 147ea35a510e0611c06e8cd7e5c7137eb32d6046 | tar -x -C "$work/release"
ln -s "$PWD/node_modules" "$work/release/node_modules"
npm --prefix "$work/release" run build
npm run build
npm run build:go
python3 benchmarks/posthog.py "$work/posthog" "$work/go-results" \
    --engines native go --native-cli "$work/release/dist/src/cli/typegen.js"
```

## Portable forms plugin

All 168 JavaScript fallback logics used the `forms` builder from kea-forms 3.2.0.
Its global hook also appeared in the toolbar extension's `resetContext`.
In a separately instrumented original check, the 168 forms builder invocations
took 0.096 s total. Visiting their complete files through JavaScript took
8.71 s, including the forms invocations. Opening the second TypeScript
program took another 0.98 s. Across the adapter's lifetime the compiler
recorded 39,355 synchronous requests and 89.9 MB of responses.

The paired runs below used the same disposable checkout, compiler, CLI,
dependencies and inputs. The only package additions to the PostHog fixture
were the new `kea-forms/lib/typegen.json` and `typegen.wasm`. Both paths
generated the forms types. Each timing includes startup, checking, plugin
execution, formatting and comparison against the checked-in files.

| Forms implementation with Go typegen | Full check runs (s) | Median | Median peak RSS | Go / JS logics |
| --- | --- | ---: | ---: | ---: |
| Shipped JavaScript visitor | 29.280, 29.031, 28.549 | 29.03 s | 5.25 GiB | 1,557 / 168 |
| Portable Go visitor | 20.723, 21.119, 20.747 | 20.75 s | 5.85 GiB | 1,725 / 0 |

The portable plugin saved 8.28 s per full check (28.5%). Its median sampled peak
memory was 0.60 GiB higher. The generated declarations agreed structurally
for 1,724 logics. In `proxyLogic`, moving the rest of the file to Go also
preserved the explicit loader payload type `ProxyRecord['id']`, which the
JavaScript printer had rendered as `ProxyRecord[]`. The checks flagged 285
and 284 files respectively.

To repeat the comparison on a dependency-installed disposable PostHog
checkout, build the sibling forms plugin, then add its portable files and
run the comparison from the typegen checkout.

```sh
npm --prefix ../kea-forms run build:typegen
cp ../kea-forms/lib/typegen.{json,wasm} "$work/posthog/frontend/node_modules/kea-forms/lib/"
python3 benchmarks/posthog.py "$work/posthog" "$work/forms-results" --engines go-js-plugins go
```
