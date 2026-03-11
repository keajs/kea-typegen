#!/usr/bin/env node

const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { spawnSync } = require('node:child_process')

function parseArgs(argv) {
    const options = {
        command: 'write',
        iterations: 5,
        warmup: 1,
    }

    for (let i = 0; i < argv.length; i++) {
        const arg = argv[i]
        if ((arg === '--command' || arg === '-c') && argv[i + 1]) {
            options.command = argv[++i]
        } else if ((arg === '--iterations' || arg === '-n') && argv[i + 1]) {
            options.iterations = Number(argv[++i])
        } else if ((arg === '--warmup' || arg === '-w') && argv[i + 1]) {
            options.warmup = Number(argv[++i])
        } else if (arg === '--help' || arg === '-h') {
            printHelp()
            process.exit(0)
        } else {
            throw new Error(`Unknown argument: ${arg}`)
        }
    }

    if (!['check', 'write'].includes(options.command)) {
        throw new Error(`Unsupported command "${options.command}". Use "check" or "write".`)
    }
    if (!Number.isInteger(options.iterations) || options.iterations <= 0) {
        throw new Error(`Invalid iterations value: ${options.iterations}`)
    }
    if (!Number.isInteger(options.warmup) || options.warmup < 0) {
        throw new Error(`Invalid warmup value: ${options.warmup}`)
    }

    return options
}

function printHelp() {
    console.log(`Usage: node ./scripts/benchmark-sample-typegen.js [options]

Options:
  -c, --command <check|write>   Command to benchmark (default: write)
  -n, --iterations <count>      Measured iterations (default: 5)
  -w, --warmup <count>          Warmup iterations per implementation (default: 1)
  -h, --help                    Show this help
`)
}

function run(command, args, options = {}) {
    const result = spawnSync(command, args, {
        cwd: options.cwd,
        env: { ...process.env, ...options.env },
        stdio: 'pipe',
        encoding: 'utf8',
    })
    if (result.status !== 0) {
        const details = [result.stdout, result.stderr].filter(Boolean).join('\n').trim()
        throw new Error(`${command} ${args.join(' ')} failed${details ? `\n${details}` : ''}`)
    }
}

function timedRun(command, args, options = {}) {
    const start = process.hrtime.bigint()
    run(command, args, options)
    const end = process.hrtime.bigint()
    return Number(end - start) / 1e6
}

function average(values) {
    return values.reduce((sum, value) => sum + value, 0) / values.length
}

function median(values) {
    const sorted = [...values].sort((a, b) => a - b)
    const middle = Math.floor(sorted.length / 2)
    if (sorted.length % 2 === 0) {
        return (sorted[middle - 1] + sorted[middle]) / 2
    }
    return sorted[middle]
}

function formatMs(value) {
    return `${value.toFixed(1)} ms`
}

function formatRuns(values) {
    return values.map((value) => value.toFixed(1)).join(', ')
}

function benchmarkImplementation(name, iterations, warmup, runOnce) {
    for (let i = 0; i < warmup; i++) {
        runOnce(`warmup-${i + 1}`)
    }

    const timings = []
    for (let i = 0; i < iterations; i++) {
        timings.push(runOnce(`run-${i + 1}`))
    }

    return {
        name,
        timings,
        mean: average(timings),
        median: median(timings),
        min: Math.min(...timings),
        max: Math.max(...timings),
    }
}

function main() {
    const options = parseArgs(process.argv.slice(2))
    const repoRoot = path.resolve(__dirname, '..')
    const samplesRoot = path.join(repoRoot, 'samples')
    const rewriteRoot = path.join(repoRoot, 'rewrite')
    const benchRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-bench-'))
    const goBinary = path.join(benchRoot, 'kea-typegen-go')
    const goCache = path.join(os.tmpdir(), 'kea-typegen-gocache-bench')
    const goModCache = path.join(os.tmpdir(), 'kea-typegen-gomodcache-bench')

    fs.mkdirSync(goCache, { recursive: true })
    fs.mkdirSync(goModCache, { recursive: true })

    run('go', ['build', '-o', goBinary, './cmd/kea-typegen-go'], {
        cwd: rewriteRoot,
        env: {
            GOCACHE: goCache,
            GOMODCACHE: goModCache,
        },
    })

    const tsRunner = (suffix) => {
        const outDir = path.join(benchRoot, `ts-${suffix}`)
        fs.mkdirSync(outDir, { recursive: true })
        const args = ['./src/cli/typegen.ts', options.command, '-r', samplesRoot, '-q']
        if (options.command === 'write') {
            args.push('-t', outDir, '--delete', '--no-import')
        }
        return timedRun('./node_modules/.bin/ts-node', args, { cwd: repoRoot })
    }

    const goRunner = (suffix) => {
        const outDir = path.join(benchRoot, `go-${suffix}`)
        fs.mkdirSync(outDir, { recursive: true })
        const args = [options.command, '-r', samplesRoot, '-q']
        if (options.command === 'write') {
            args.push('-t', outDir, '--delete', '--no-import')
        }
        return timedRun(goBinary, args, {
            cwd: rewriteRoot,
            env: {
                GOCACHE: goCache,
                GOMODCACHE: goModCache,
            },
        })
    }

    const tsStats = benchmarkImplementation('TypeScript', options.iterations, options.warmup, tsRunner)
    const goStats = benchmarkImplementation('Go', options.iterations, options.warmup, goRunner)
    const speedup = tsStats.mean / goStats.mean
    const faster = speedup > 1
    const timeDeltaPercent = faster ? (1 - goStats.mean / tsStats.mean) * 100 : (goStats.mean / tsStats.mean - 1) * 100

    console.log(`Benchmark command: ${options.command}`)
    console.log(`Samples root: ${samplesRoot}`)
    console.log(`Warmup iterations: ${options.warmup}`)
    console.log(`Measured iterations: ${options.iterations}`)
    console.log('')
    console.log(
        `TypeScript mean: ${formatMs(tsStats.mean)} (median ${formatMs(tsStats.median)}, min ${formatMs(tsStats.min)}, max ${formatMs(tsStats.max)})`,
    )
    console.log(`TypeScript runs: ${formatRuns(tsStats.timings)}`)
    console.log(
        `Go mean: ${formatMs(goStats.mean)} (median ${formatMs(goStats.median)}, min ${formatMs(goStats.min)}, max ${formatMs(goStats.max)})`,
    )
    console.log(`Go runs: ${formatRuns(goStats.timings)}`)
    console.log('')
    if (faster) {
        console.log(`Go is ${speedup.toFixed(2)}x faster on mean runtime (${timeDeltaPercent.toFixed(1)}% less time).`)
    } else {
        console.log(
            `Go is ${(1 / speedup).toFixed(2)}x slower on mean runtime (${timeDeltaPercent.toFixed(1)}% more time).`,
        )
    }
    console.log(`Benchmark workspace: ${benchRoot}`)
}

try {
    main()
} catch (error) {
    console.error(error.message || String(error))
    process.exit(1)
}
