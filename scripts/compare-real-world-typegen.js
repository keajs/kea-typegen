#!/usr/bin/env node

const crypto = require('node:crypto')
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
      --js-mode <mode>     JS CLI mode: auto, dist, or source
      --file <path>        Compare a generated Type file or source logic file; repeatable
      --files-from <file>  Read selected paths from a JSON array or newline-delimited file
      --keep-worktrees     Keep temporary worktrees after the run
      --json               Print the final summary as JSON
  -h, --help               Show this help
`)
}

function parseArgs(argv) {
    const options = {
        target: '',
        repo: '',
        jsMode: 'source',
        files: [],
        filesFrom: '',
        keepWorktrees: false,
        json: false,
    }

    for (let i = 0; i < argv.length; i++) {
        const arg = argv[i]
        if (arg === '--target' && argv[i + 1]) {
            options.target = argv[++i]
        } else if (arg === '--repo' && argv[i + 1]) {
            options.repo = argv[++i]
        } else if (arg === '--js-mode' && argv[i + 1]) {
            options.jsMode = argv[++i]
        } else if (arg === '--file' && argv[i + 1]) {
            options.files.push(String(argv[++i]).replace(/\\/g, '/'))
        } else if (arg === '--files-from' && argv[i + 1]) {
            options.filesFrom = path.resolve(argv[++i])
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
    if (!['auto', 'dist', 'source'].includes(options.jsMode)) {
        throw new Error(`Unknown --js-mode: ${options.jsMode}`)
    }
    if (options.filesFrom) {
        options.files.push(...readSelectedFiles(options.filesFrom))
    }
    options.files = [...new Set(options.files.map((file) => file.replace(/\\/g, '/')))].sort()

    return options
}

function readSelectedFiles(filePath) {
    const text = fs.readFileSync(filePath, 'utf8').trim()
    if (!text) {
        return []
    }
    if (text.startsWith('[')) {
        const parsed = JSON.parse(text)
        if (!Array.isArray(parsed)) {
            throw new Error(`Expected ${filePath} to contain a JSON array`)
        }
        return parsed.map((file) => String(file).replace(/\\/g, '/'))
    }
    return text
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => line && !line.startsWith('#'))
        .map((file) => file.replace(/\\/g, '/'))
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

function hashFile(filePath) {
    return crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex')
}

function describeFile(filePath) {
    const stat = fs.statSync(filePath)
    return {
        path: filePath,
        sha256: hashFile(filePath),
        size: stat.size,
        mtimeMs: stat.mtimeMs,
    }
}

function resolveJSEntrypoint(repoRoot, requestedMode) {
    const wrapperPath = path.join(repoRoot, 'bin', 'kea-typegen-js')
    const distPath = path.join(repoRoot, 'dist', 'src', 'cli', 'typegen.js')
    const sourcePath = path.join(repoRoot, 'src', 'cli', 'typegen.ts')
    const tsNodePath = path.join(repoRoot, 'node_modules', '.bin', 'ts-node')

    let selectedMode = requestedMode
    if (requestedMode === 'auto') {
        if (fs.existsSync(distPath)) {
            selectedMode = 'dist'
        } else if (fs.existsSync(tsNodePath)) {
            selectedMode = 'source'
        } else {
            throw new Error('Could not resolve JS entrypoint: no compiled CLI or ts-node available')
        }
    }

    if (selectedMode === 'dist') {
        if (!fs.existsSync(distPath)) {
            throw new Error(`Compiled JS CLI not found: ${distPath}`)
        }
        return {
            requestedMode,
            selectedMode,
            wrapper: describeFile(wrapperPath),
            cli: describeFile(distPath),
        }
    }

    if (!fs.existsSync(tsNodePath)) {
        throw new Error(`ts-node not found: ${tsNodePath}`)
    }

    return {
        requestedMode,
        selectedMode: 'source',
        wrapper: describeFile(wrapperPath),
        cli: describeFile(sourcePath),
        runner: describeFile(tsNodePath),
    }
}

function readJSON(filePath) {
    return JSON.parse(fs.readFileSync(filePath, 'utf8'))
}

function gitHead(repoPath) {
    return run('git', ['-C', repoPath, 'rev-parse', 'HEAD']).stdout.trim()
}

function ensurePrepared(repoRoot, jsMode) {
    const preparedJS = path.join(repoRoot, 'dist', 'src', 'cli', 'typegen.js')
    const tsNode = path.join(repoRoot, 'node_modules', '.bin', 'ts-node')
    const preparedGo = path.join(repoRoot, 'rewrite', 'bin', 'kea-typegen-go')

    const needsJS =
        (jsMode === 'dist' && !fs.existsSync(preparedJS)) ||
        (jsMode === 'source' && !fs.existsSync(tsNode)) ||
        (jsMode === 'auto' && !fs.existsSync(preparedJS) && !fs.existsSync(tsNode))
    if (needsJS) {
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

const SOURCE_EXTENSIONS = ['.ts', '.tsx', '.mts', '.cts', '.js', '.jsx', '.mjs', '.cjs']

function isSourceFilePath(filePath) {
    return SOURCE_EXTENSIONS.some((extension) => filePath.endsWith(extension)) && !filePath.endsWith('Type.ts')
}

function typePathForSourcePath(sourcePath) {
    for (const extension of SOURCE_EXTENSIONS) {
        if (sourcePath.endsWith(extension)) {
            return `${sourcePath.slice(0, -extension.length)}Type.ts`
        }
    }
    throw new Error(`Could not derive generated Type file path for ${sourcePath}`)
}

function sourceCandidatesForTypePath(typePath) {
    if (!typePath.endsWith('Type.ts')) {
        return []
    }
    const base = typePath.slice(0, -'Type.ts'.length)
    return SOURCE_EXTENSIONS.map((extension) => `${base}${extension}`)
}

function pathExists(root, relativePath) {
    return fs.existsSync(path.join(root, relativePath))
}

function resolveSelectedFile(targetRepoPath, targetConfig, selectedPath) {
    const normalizedPath = selectedPath.replace(/\\/g, '/').replace(/^\.\/+/, '')
    const compareRoot = path.join(targetRepoPath, targetConfig.compareRoot)
    const sourceCandidates = isSourceFilePath(normalizedPath)
        ? [normalizedPath]
        : sourceCandidatesForTypePath(normalizedPath)
    if (sourceCandidates.length === 0) {
        throw new Error(`Selected path must be a source logic file or generated Type file: ${selectedPath}`)
    }

    for (const sourceCandidate of sourceCandidates) {
        if (pathExists(compareRoot, sourceCandidate)) {
            const sourceAbs = path.join(compareRoot, sourceCandidate)
            const sourceCompareRel = path.relative(compareRoot, sourceAbs).replace(/\\/g, '/')
            const typeCompareRel = normalizedPath.endsWith('Type.ts')
                ? normalizedPath
                : typePathForSourcePath(sourceCompareRel)
            return {
                input: selectedPath,
                sourceCompareRel,
                sourceRepoRel: path.relative(targetRepoPath, sourceAbs).replace(/\\/g, '/'),
                typeCompareRel,
            }
        }
        if (pathExists(targetRepoPath, sourceCandidate)) {
            const sourceAbs = path.join(targetRepoPath, sourceCandidate)
            const sourceCompareRel = path.relative(compareRoot, sourceAbs).replace(/\\/g, '/')
            if (sourceCompareRel.startsWith('../')) {
                throw new Error(
                    `Selected source file is outside compare root ${targetConfig.compareRoot}: ${selectedPath}`,
                )
            }
            const typeCompareRel = normalizedPath.endsWith('Type.ts')
                ? path.relative(compareRoot, path.join(targetRepoPath, normalizedPath)).replace(/\\/g, '/')
                : typePathForSourcePath(sourceCompareRel)
            return {
                input: selectedPath,
                sourceCompareRel,
                sourceRepoRel: path.relative(targetRepoPath, sourceAbs).replace(/\\/g, '/'),
                typeCompareRel,
            }
        }
    }

    throw new Error(`Could not resolve selected source for ${selectedPath}`)
}

function resolveSelectedFiles(targetRepoPath, targetConfig, selectedPaths) {
    const selected = selectedPaths.map((file) => resolveSelectedFile(targetRepoPath, targetConfig, file))
    const byTypeFile = new Map()
    for (const item of selected) {
        byTypeFile.set(item.typeCompareRel, item)
    }
    return [...byTypeFile.values()].sort((left, right) => left.typeCompareRel.localeCompare(right.typeCompareRel))
}

function configArgPath(args) {
    for (let index = 0; index < args.length - 1; index++) {
        if (args[index] === '--config' || args[index] === '-c') {
            return args[index + 1]
        }
    }
    return 'tsconfig.json'
}

function argPath(args, names, fallback) {
    for (let index = 0; index < args.length - 1; index++) {
        if (names.includes(args[index])) {
            return args[index + 1]
        }
    }
    return fallback
}

function hasArg(args, names) {
    return args.some((arg) => names.includes(arg))
}

function findUp(startPath, fileName) {
    let current = path.resolve(startPath)
    while (true) {
        const candidate = path.join(current, fileName)
        if (fs.existsSync(candidate)) {
            return candidate
        }
        const parent = path.dirname(current)
        if (parent === current) {
            return ''
        }
        current = parent
    }
}

function resolvedRenderPaths(worktreeRoot, targetConfig) {
    const cwd = path.join(worktreeRoot, targetConfig.goCwd)
    const config = path.resolve(cwd, configArgPath(targetConfig.goArgs))
    const root = path.resolve(cwd, argPath(targetConfig.goArgs, ['--root', '-r'], path.dirname(config)))
    const types = path.resolve(cwd, argPath(targetConfig.goArgs, ['--types', '-t'], root))
    return {
        cwd,
        config,
        root,
        types,
        packageJson: findUp(root, 'package.json') || findUp(path.dirname(config), 'package.json'),
    }
}

function writeSlicedConfig(worktreeRoot, targetConfig, selectedFiles) {
    if (selectedFiles.length === 0) {
        return ''
    }

    const baseConfigPath = path.resolve(path.join(worktreeRoot, targetConfig.goCwd, configArgPath(targetConfig.goArgs)))
    const configDir = path.dirname(baseConfigPath)
    const sliceConfigPath = path.join(configDir, 'tsconfig.kea-typegen-slice.json')
    const files = selectedFiles.map((file) =>
        path.relative(configDir, path.join(worktreeRoot, file.sourceRepoRel)).replace(/\\/g, '/'),
    )
    fs.writeFileSync(
        sliceConfigPath,
        JSON.stringify(
            {
                extends: `./${path.basename(baseConfigPath)}`,
                files,
                include: [],
            },
            null,
            2,
        ),
    )
    return sliceConfigPath
}

function withConfigArg(args, configPath) {
    if (!configPath) {
        return [...args]
    }
    const nextArgs = [...args]
    for (let index = 0; index < nextArgs.length - 1; index++) {
        if (nextArgs[index] === '--config' || nextArgs[index] === '-c') {
            nextArgs[index + 1] = configPath
            return nextArgs
        }
    }
    return [...nextArgs, '--config', configPath]
}

function runSelectedJSRender(repoRoot, targetConfig, worktreeRoot, selectedFile, jsMode, jsPluginManifestPath) {
    const paths = resolvedRenderPaths(worktreeRoot, targetConfig)
    const renderArgs = [
        path.join(repoRoot, 'scripts', 'render-js-typegen.js'),
        '--config',
        paths.config,
        '--root',
        paths.root,
        '--types',
        paths.types,
        '--file',
        path.join(worktreeRoot, selectedFile.sourceRepoRel),
    ]
    if (paths.packageJson) {
        renderArgs.push('--package-json', paths.packageJson)
    }
    if (hasArg(targetConfig.jsArgs, ['--add-ts-nocheck'])) {
        renderArgs.push('--add-ts-nocheck')
    }
    if (hasArg(targetConfig.jsArgs, ['--import-global-types'])) {
        renderArgs.push('--import-global-types')
    }
    run('node', renderArgs, {
        cwd: path.join(worktreeRoot, targetConfig.jsCwd),
        env: {
            KEA_TYPEGEN_JS_MODE: jsMode,
            KEA_TYPEGEN_DEBUG_PLUGIN_MANIFEST: jsPluginManifestPath,
            ...targetConfig.jsEnv,
        },
    })
}

function runSelectedGoRender(repoRoot, targetConfig, worktreeRoot, selectedFile) {
    const paths = resolvedRenderPaths(worktreeRoot, targetConfig)
    const goCli = path.join(repoRoot, 'rewrite', 'bin', 'kea-typegen-go')
    const result = run(
        goCli,
        [
            'render',
            '--config',
            paths.config,
            '--root',
            paths.root,
            '--types',
            paths.types,
            '--file',
            path.join(worktreeRoot, selectedFile.sourceRepoRel),
            '--format',
            'typegen',
        ],
        {
            cwd: repoRoot,
            env: { KEA_TYPEGEN_CWD: paths.cwd, ...(targetConfig.goEnv || {}) },
        },
    )
    const output = result.stdout
    if (output.trim() === '') {
        return
    }
    const typeFile = path.join(worktreeRoot, targetConfig.compareRoot, selectedFile.typeCompareRel)
    fs.mkdirSync(path.dirname(typeFile), { recursive: true })
    fs.writeFileSync(typeFile, output)
}

function addWorktree(repoPath, worktreePath) {
    run('git', ['-C', repoPath, 'worktree', 'add', '--detach', worktreePath, 'HEAD'])
}

function removeWorktree(repoPath, worktreePath) {
    run('git', ['-C', repoPath, 'worktree', 'remove', '--force', worktreePath], { expectedStatuses: [0, 128] })
}

function runTypegenPair(repoRoot, targetConfig, targetRepoPath, worktreeRoot, jsMode, selectedFiles = []) {
    const jsWorktree = path.join(worktreeRoot, `${targetConfig.label}-js`)
    const goWorktree = path.join(worktreeRoot, `${targetConfig.label}-go`)
    const jsPluginManifestPath = path.join(worktreeRoot, `${targetConfig.label}-js-plugins.json`)
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

        let jsSliceConfig = ''
        let goSliceConfig = ''
        if (selectedFiles.length > 0) {
            for (const selectedFile of selectedFiles) {
                runSelectedJSRender(repoRoot, targetConfig, jsWorktree, selectedFile, jsMode, jsPluginManifestPath)
                runSelectedGoRender(repoRoot, targetConfig, goWorktree, selectedFile)
            }
        } else {
            jsSliceConfig = writeSlicedConfig(jsWorktree, targetConfig, selectedFiles)
            goSliceConfig = writeSlicedConfig(goWorktree, targetConfig, selectedFiles)

            const jsCli = path.join(repoRoot, 'bin', 'kea-typegen-js')
            const goCli = path.join(repoRoot, 'rewrite', 'bin', 'kea-typegen-go')

            run(jsCli, withConfigArg(targetConfig.jsArgs, jsSliceConfig), {
                cwd: path.join(jsWorktree, targetConfig.jsCwd),
                env: {
                    KEA_TYPEGEN_JS_MODE: jsMode,
                    KEA_TYPEGEN_DEBUG_PLUGIN_MANIFEST: jsPluginManifestPath,
                    ...targetConfig.jsEnv,
                },
            })
            run(goCli, withConfigArg(targetConfig.goArgs, goSliceConfig), {
                cwd: repoRoot,
                env: { KEA_TYPEGEN_CWD: path.join(goWorktree, targetConfig.goCwd), ...(targetConfig.goEnv || {}) },
            })
        }

        if (!fs.existsSync(jsPluginManifestPath)) {
            throw new Error(`JS plugin manifest was not written: ${jsPluginManifestPath}`)
        }

        return {
            jsWorktree,
            goWorktree,
            jsPluginManifestPath,
            jsPluginManifest: readJSON(jsPluginManifestPath),
            jsSliceConfig,
            goSliceConfig,
            cleanup,
        }
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
    const selectedFiles = resolveSelectedFiles(targetRepoPath, targetConfig, options.files)

    ensurePrepared(repoRoot, options.jsMode)
    const jsEntrypoint = resolveJSEntrypoint(repoRoot, options.jsMode)
    const goBinary = describeFile(path.join(repoRoot, 'rewrite', 'bin', 'kea-typegen-go'))

    const worktreeRoot = fs.mkdtempSync(path.join(os.tmpdir(), `${targetConfig.label}-corpus-`))
    const cleanup = []

    try {
        const pair = runTypegenPair(repoRoot, targetConfig, targetRepoPath, worktreeRoot, options.jsMode, selectedFiles)
        cleanup.push(...pair.cleanup)

        const jsCompareDir = path.join(pair.jsWorktree, targetConfig.compareRoot)
        const goCompareDir = path.join(pair.goWorktree, targetConfig.compareRoot)
        const htmlOut = path.join(worktreeRoot, `${targetConfig.label}-compare.html`)
        const baselineManifest = path.join(worktreeRoot, `${targetConfig.label}-baseline.json`)
        const compareArgs = [
            path.join(repoRoot, 'scripts', 'compare-generated-typegen.js'),
            '--ts-dir',
            jsCompareDir,
            '--go-dir',
            goCompareDir,
            '--json',
            '--html-out',
            htmlOut,
        ]
        for (const file of selectedFiles) {
            compareArgs.push('--file', file.typeCompareRel)
        }

        const compare = run('node', compareArgs, { cwd: repoRoot })

        const summary = JSON.parse(compare.stdout)
        const baseline = {
            generatedAt: new Date().toISOString(),
            repoRoot,
            repoHead: gitHead(repoRoot),
            targetRepoPath,
            targetRepoHead: gitHead(targetRepoPath),
            jsEntrypoint,
            jsPluginManifestPath: pair.jsPluginManifestPath,
            jsPluginManifest: pair.jsPluginManifest,
            selectedFiles,
            jsSliceConfig: pair.jsSliceConfig || null,
            goSliceConfig: pair.goSliceConfig || null,
            goBinary,
        }
        fs.writeFileSync(baselineManifest, JSON.stringify(baseline, null, 2))
        const report = {
            target: targetConfig.label,
            repoPath: targetRepoPath,
            worktreeRoot,
            htmlReport: htmlOut,
            baselineManifest,
            baseline,
            summary,
        }

        if (options.json) {
            console.log(JSON.stringify(report, null, 2))
        } else {
            console.log(`Target: ${targetConfig.label}`)
            console.log(`Repo: ${targetRepoPath}`)
            console.log(
                `JS mode: ${baseline.jsEntrypoint.selectedMode} (requested: ${baseline.jsEntrypoint.requestedMode})`,
            )
            console.log(`JS entrypoint: ${baseline.jsEntrypoint.cli.path}`)
            if (selectedFiles.length > 0) {
                console.log(`Selected files: ${selectedFiles.map((file) => file.typeCompareRel).join(', ')}`)
            }
            console.log(`Comparable files: ${summary.comparableFiles}`)
            console.log(
                `Semantic matches: ${summary.semanticMatches}/${summary.totalFiles} (${summary.semanticAccuracy.toFixed(2)}%)`,
            )
            console.log(
                `Exact matches: ${summary.exactMatches}/${summary.totalFiles} (${summary.exactAccuracy.toFixed(2)}%)`,
            )
            console.log(`TS_ONLY: ${summary.tsOnly.length}`)
            console.log(`GO_ONLY: ${summary.goOnly.length}`)
            console.log(`Semantic diffs: ${summary.semanticDiffs.length}`)
            if (summary.semanticDiffs.length > 0) {
                console.log(`Top semantic diffs: ${summary.semanticDiffs.slice(0, 10).join(', ')}`)
            }
            console.log(`HTML report: ${htmlOut}`)
            console.log(`Baseline manifest: ${baselineManifest}`)
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
