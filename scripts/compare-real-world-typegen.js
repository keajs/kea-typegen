#!/usr/bin/env node

const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { spawnSync } = require('node:child_process')

const TARGETS = {
    frameos: {
        label: 'frameos',
        repoPath: '../frameos',
        jsCwd: 'frontend',
        goCwd: 'frontend',
        compareRoot: 'frontend',
        jsArgs: ['write', '--write-paths', '-q'],
        jsEnv: {},
        goArgs: ['write', '--config', 'tsconfig.json', '--root', '.', '--write-paths', '-q'],
        goEnv: { KEA_TYPEGEN_PARITY_MODE: '1' },
    },
    posthog: {
        label: 'posthog',
        repoPath: '../posthog',
        jsCwd: '.',
        goCwd: '.',
        compareRoot: '.',
        jsArgs: ['write', '--delete', '--show-ts-errors', '-q'],
        jsEnv: { NODE_OPTIONS: '--max-old-space-size=16384' },
        goArgs: [
            'write',
            '--config',
            'tsconfig.json',
            '--root',
            'frontend/src',
            '--types',
            'frontend/src',
            '--write-paths',
            '--delete',
            '--show-ts-errors',
            '-q',
        ],
        goEnv: { KEA_TYPEGEN_PARITY_MODE: '1' },
    },
}

function printHelp() {
    console.log(`Usage: node ./scripts/compare-real-world-typegen.js --target <frameos|posthog> [options]

Options:
      --target <name>      Named compare target
      --repo <path>        Override target repository path
      --keep-worktrees     Keep temporary worktrees after the run
      --json               Print the final summary as JSON
  -h, --help               Show this help
`)
}

function parseArgs(argv) {
    const options = {
        target: '',
        repo: '',
        keepWorktrees: false,
        json: false,
    }

    for (let i = 0; i < argv.length; i++) {
        const arg = argv[i]
        if (arg === '--target' && argv[i + 1]) {
            options.target = argv[++i]
        } else if (arg === '--repo' && argv[i + 1]) {
            options.repo = argv[++i]
        } else if (arg === '--keep-worktrees') {
            options.keepWorktrees = true
        } else if (arg === '--json') {
            options.json = true
        } else if (arg === '--help' || arg === '-h') {
            printHelp()
            process.exit(0)
        } else {
            throw new Error(`Unknown argument: ${arg}`)
        }
    }

    if (!options.target) {
        throw new Error('Missing --target')
    }
    if (!TARGETS[options.target]) {
        throw new Error(`Unknown target: ${options.target}`)
    }

    return options
}

function run(command, args, options = {}) {
    const result = spawnSync(command, args, {
        cwd: options.cwd,
        env: { ...process.env, ...options.env },
        stdio: 'pipe',
        encoding: 'utf8',
    })
    const expectedStatuses = options.expectedStatuses || [0]
    if (!expectedStatuses.includes(result.status)) {
        const details = [result.stdout, result.stderr].filter(Boolean).join('\n').trim()
        throw new Error(`${command} ${args.join(' ')} failed${details ? `\n${details}` : ''}`)
    }
    return result
}

function ensurePrepared(repoRoot) {
    const preparedJS = path.join(repoRoot, 'dist', 'src', 'cli', 'typegen.js')
    const tsNode = path.join(repoRoot, 'node_modules', '.bin', 'ts-node')
    const preparedGo = path.join(repoRoot, 'rewrite', 'bin', 'kea-typegen-go')

    if (!fs.existsSync(preparedJS) && !fs.existsSync(tsNode)) {
        run(path.join(repoRoot, 'bin', 'prepare-js'), [], { cwd: repoRoot })
    }
    if (!fs.existsSync(preparedGo)) {
        run(path.join(repoRoot, 'bin', 'prepare-go'), [], { cwd: repoRoot })
    }
}

function listNodeModulesDirs(root) {
    const matches = []

    function visit(current, relativePath) {
        for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
            if (entry.name === '.git') {
                continue
            }

            const fullPath = path.join(current, entry.name)
            const nextRelativePath = relativePath ? path.join(relativePath, entry.name) : entry.name

            if (entry.name === 'node_modules' && entry.isDirectory()) {
                matches.push(nextRelativePath)
                continue
            }
            if (entry.isDirectory()) {
                visit(fullPath, nextRelativePath)
            }
        }
    }

    visit(root, '')
    return matches.sort()
}

function linkDependencyDirs(sourceRoot, worktreeRoot) {
    for (const relativeDir of listNodeModulesDirs(sourceRoot)) {
        const sourcePath = path.join(sourceRoot, relativeDir)
        const destinationPath = path.join(worktreeRoot, relativeDir)
        fs.mkdirSync(path.dirname(destinationPath), { recursive: true })
        if (fs.existsSync(destinationPath)) {
            fs.rmSync(destinationPath, { recursive: true, force: true })
        }
        fs.symlinkSync(sourcePath, destinationPath, 'dir')
    }
}

function removeGeneratedArtifacts(root) {
    function visit(directory) {
        for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
            const fullPath = path.join(directory, entry.name)
            if (entry.isDirectory()) {
                if (entry.name === '.typegen') {
                    fs.rmSync(fullPath, { recursive: true, force: true })
                    continue
                }
                visit(fullPath)
                continue
            }
            if (entry.isFile() && entry.name.endsWith('Type.ts')) {
                fs.rmSync(fullPath, { force: true })
            }
        }
    }

    visit(root)
}

function addWorktree(repoPath, worktreePath) {
    run('git', ['-C', repoPath, 'worktree', 'add', '--detach', worktreePath, 'HEAD'])
}

function removeWorktree(repoPath, worktreePath) {
    run('git', ['-C', repoPath, 'worktree', 'remove', '--force', worktreePath], { expectedStatuses: [0, 128] })
}

function runTypegenPair(repoRoot, targetConfig, targetRepoPath, worktreeRoot) {
    const jsWorktree = path.join(worktreeRoot, `${targetConfig.label}-js`)
    const goWorktree = path.join(worktreeRoot, `${targetConfig.label}-go`)
    const cleanup = []

    try {
        addWorktree(targetRepoPath, jsWorktree)
        cleanup.push(() => removeWorktree(targetRepoPath, jsWorktree))

        addWorktree(targetRepoPath, goWorktree)
        cleanup.push(() => removeWorktree(targetRepoPath, goWorktree))

        linkDependencyDirs(targetRepoPath, jsWorktree)
        linkDependencyDirs(targetRepoPath, goWorktree)

        removeGeneratedArtifacts(jsWorktree)
        removeGeneratedArtifacts(goWorktree)

        const jsCli = path.join(repoRoot, 'bin', 'kea-typegen-js')
        const goCli = path.join(repoRoot, 'rewrite', 'bin', 'kea-typegen-go')

        run(jsCli, targetConfig.jsArgs, {
            cwd: path.join(jsWorktree, targetConfig.jsCwd),
            env: targetConfig.jsEnv,
        })
        run(goCli, targetConfig.goArgs, {
            cwd: repoRoot,
            env: { KEA_TYPEGEN_CWD: path.join(goWorktree, targetConfig.goCwd), ...(targetConfig.goEnv || {}) },
        })

        return { jsWorktree, goWorktree, cleanup }
    } catch (error) {
        while (cleanup.length > 0) {
            try {
                cleanup.pop()()
            } catch {}
        }
        throw error
    }
}

function main() {
    const options = parseArgs(process.argv.slice(2))
    const repoRoot = path.resolve(__dirname, '..')
    const targetConfig = TARGETS[options.target]
    const targetRepoPath = path.resolve(repoRoot, options.repo || targetConfig.repoPath)

    ensurePrepared(repoRoot)

    const worktreeRoot = fs.mkdtempSync(path.join(os.tmpdir(), `${targetConfig.label}-corpus-`))
    const cleanup = []

    try {
        const pair = runTypegenPair(repoRoot, targetConfig, targetRepoPath, worktreeRoot)
        cleanup.push(...pair.cleanup)

        const jsCompareDir = path.join(pair.jsWorktree, targetConfig.compareRoot)
        const goCompareDir = path.join(pair.goWorktree, targetConfig.compareRoot)
        const htmlOut = path.join(worktreeRoot, `${targetConfig.label}-compare.html`)

        const compare = run(
            'node',
            [
                path.join(repoRoot, 'scripts', 'compare-generated-typegen.js'),
                '--ts-dir',
                jsCompareDir,
                '--go-dir',
                goCompareDir,
                '--json',
                '--html-out',
                htmlOut,
            ],
            { cwd: repoRoot },
        )

        const summary = JSON.parse(compare.stdout)
        const report = {
            target: targetConfig.label,
            repoPath: targetRepoPath,
            worktreeRoot,
            htmlReport: htmlOut,
            summary,
        }

        if (options.json) {
            console.log(JSON.stringify(report, null, 2))
        } else {
            console.log(`Target: ${targetConfig.label}`)
            console.log(`Repo: ${targetRepoPath}`)
            console.log(`Comparable files: ${summary.comparableFiles}`)
            console.log(`Semantic matches: ${summary.semanticMatches}/${summary.totalFiles} (${summary.semanticAccuracy.toFixed(2)}%)`)
            console.log(`Exact matches: ${summary.exactMatches}/${summary.totalFiles} (${summary.exactAccuracy.toFixed(2)}%)`)
            console.log(`TS_ONLY: ${summary.tsOnly.length}`)
            console.log(`GO_ONLY: ${summary.goOnly.length}`)
            console.log(`Semantic diffs: ${summary.semanticDiffs.length}`)
            if (summary.semanticDiffs.length > 0) {
                console.log(`Top semantic diffs: ${summary.semanticDiffs.slice(0, 10).join(', ')}`)
            }
            console.log(`HTML report: ${htmlOut}`)
        }

        if (!options.keepWorktrees) {
            while (cleanup.length > 0) {
                cleanup.pop()()
            }
            fs.rmSync(worktreeRoot, { recursive: true, force: true })
        }
    } catch (error) {
        while (cleanup.length > 0) {
            try {
                cleanup.pop()()
            } catch {}
        }
        if (!options.keepWorktrees) {
            fs.rmSync(worktreeRoot, { recursive: true, force: true })
        }
        throw error
    }
}

try {
    main()
} catch (error) {
    console.error(error.message || String(error))
    process.exit(1)
}
