#!/usr/bin/env node

const path = require('node:path')

const { compareOutputs } = require('./benchmark-sample-typegen.js')

function printHelp() {
    console.log(`Usage: node ./scripts/compare-generated-typegen.js --ts-dir <dir> --go-dir <dir> [options]

Options:
      --ts-dir <dir>     Directory containing the TypeScript-generated files
      --go-dir <dir>     Directory containing the Go-generated files
      --top <count>      Number of notable diffs to print (default: 20)
      --json             Print the full summary as JSON
  -h, --help             Show this help
`)
}

function parseArgs(argv) {
    const options = {
        tsDir: '',
        goDir: '',
        top: 20,
        json: false,
    }

    for (let i = 0; i < argv.length; i++) {
        const arg = argv[i]
        if (arg === '--ts-dir' && argv[i + 1]) {
            options.tsDir = path.resolve(argv[++i])
        } else if (arg === '--go-dir' && argv[i + 1]) {
            options.goDir = path.resolve(argv[++i])
        } else if (arg === '--top' && argv[i + 1]) {
            options.top = Number(argv[++i])
        } else if (arg === '--json') {
            options.json = true
        } else if (arg === '--help' || arg === '-h') {
            printHelp()
            process.exit(0)
        } else {
            throw new Error(`Unknown argument: ${arg}`)
        }
    }

    if (!options.tsDir) {
        throw new Error('Missing --ts-dir')
    }
    if (!options.goDir) {
        throw new Error('Missing --go-dir')
    }
    if (!Number.isInteger(options.top) || options.top < 0) {
        throw new Error(`Invalid --top value: ${options.top}`)
    }

    return options
}

function formatPercent(value) {
    return `${value.toFixed(2)}%`
}

function main() {
    const options = parseArgs(process.argv.slice(2))
    const summary = compareOutputs(options.tsDir, options.goDir)

    if (options.json) {
        console.log(JSON.stringify(summary, null, 2))
        return
    }

    console.log(`TS dir: ${options.tsDir}`)
    console.log(`Go dir: ${options.goDir}`)
    console.log(`Total files: ${summary.totalFiles}`)
    console.log(`Comparable files: ${summary.comparableFiles}`)
    console.log(`Semantic matches: ${summary.semanticMatches} (${formatPercent(summary.semanticAccuracy)})`)
    console.log(`Exact matches: ${summary.exactMatches} (${formatPercent(summary.exactAccuracy)})`)
    console.log(`TS_ONLY: ${summary.tsOnly.length}`)
    console.log(`GO_ONLY: ${summary.goOnly.length}`)
    console.log(`Semantic diffs: ${summary.semanticDiffs.length}`)

    const noteworthy = [
        ...summary.tsOnly.map((file) => `TS_ONLY ${file}`),
        ...summary.goOnly.map((file) => `GO_ONLY ${file}`),
        ...summary.semanticDiffs.map((file) => `DIFF ${file}`),
    ].slice(0, options.top)

    if (noteworthy.length > 0) {
        console.log(`Top differences: ${noteworthy.join(', ')}`)
    }
}

try {
    main()
} catch (error) {
    console.error(error.message || String(error))
    process.exit(1)
}
