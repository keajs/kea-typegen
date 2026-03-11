#!/usr/bin/env node

const fs = require('node:fs')
const path = require('node:path')
const { spawnSync } = require('node:child_process')
const ts = require('typescript')

const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed, removeComments: true })

function parseArgs(argv) {
    const options = {
        command: 'write',
        iterations: 5,
        warmup: 1,
        keepWorkspace: false,
        skipPrepare: false,
        samples: undefined,
    }

    for (let i = 0; i < argv.length; i++) {
        const arg = argv[i]
        if ((arg === '--command' || arg === '-c') && argv[i + 1]) {
            options.command = argv[++i]
        } else if ((arg === '--iterations' || arg === '-n') && argv[i + 1]) {
            options.iterations = Number(argv[++i])
        } else if ((arg === '--warmup' || arg === '-w') && argv[i + 1]) {
            options.warmup = Number(argv[++i])
        } else if (arg === '--samples' && argv[i + 1]) {
            options.samples = path.resolve(argv[++i])
        } else if (arg === '--skip-prepare') {
            options.skipPrepare = true
        } else if (arg === '--keep-workspace') {
            options.keepWorkspace = true
        } else if (arg === '--cleanup') {
            options.keepWorkspace = false
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
    console.log(`Usage: ./bin/benchmark [options]

Options:
  -c, --command <check|write>   Command to benchmark (default: write)
  -n, --iterations <count>      Measured iterations (default: 5)
  -w, --warmup <count>          Warmup iterations per implementation (default: 1)
      --samples <path>          Override the samples directory
      --skip-prepare            Assume dist/ and rewrite/bin are already prepared
      --keep-workspace          Keep the temporary benchmark workspace afterwards
      --cleanup                 Remove the temporary benchmark workspace afterwards (default)
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
    const expectedStatuses = options.expectedStatuses || [0]
    if (!expectedStatuses.includes(result.status)) {
        const details = [result.stdout, result.stderr].filter(Boolean).join('\n').trim()
        throw new Error(`${command} ${args.join(' ')} failed${details ? `\n${details}` : ''}`)
    }
    return result
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

function formatPercent(value) {
    return `${value.toFixed(1)}%`
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

function copyDirectory(source, destination) {
    fs.mkdirSync(destination, { recursive: true })
    for (const entry of fs.readdirSync(source, { withFileTypes: true })) {
        const sourcePath = path.join(source, entry.name)
        const destinationPath = path.join(destination, entry.name)
        if (entry.isDirectory()) {
            copyDirectory(sourcePath, destinationPath)
        } else if (entry.isSymbolicLink()) {
            fs.symlinkSync(fs.readlinkSync(sourcePath), destinationPath)
        } else {
            fs.copyFileSync(sourcePath, destinationPath)
        }
    }
}

function listFiles(root) {
    const files = []
    const visit = (directory) => {
        for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
            const fullPath = path.join(directory, entry.name)
            if (entry.isDirectory()) {
                visit(fullPath)
            } else if (entry.isFile()) {
                files.push(path.relative(root, fullPath))
            }
        }
    }
    if (fs.existsSync(root)) {
        visit(root)
    }
    return files.sort()
}

function listGeneratedTypeFiles(root) {
    return listFiles(root).filter((file) => file.endsWith('Type.ts'))
}

function removeGeneratedArtifacts(root) {
    const summary = {
        removedTypeFiles: 0,
        removedCacheDirs: 0,
    }

    const visit = (directory) => {
        for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
            const fullPath = path.join(directory, entry.name)
            if (entry.isDirectory()) {
                if (entry.name === '.typegen') {
                    fs.rmSync(fullPath, { recursive: true, force: true })
                    summary.removedCacheDirs += 1
                    continue
                }
                visit(fullPath)
                continue
            }
            if (entry.isFile() && entry.name.endsWith('Type.ts')) {
                fs.rmSync(fullPath, { force: true })
                summary.removedTypeFiles += 1
            }
        }
    }

    if (fs.existsSync(root)) {
        visit(root)
    }
    return summary
}

function createBenchmarkClone(repoRoot, samplesSource, label) {
    const cloneRoot = fs.mkdtempSync(path.join(repoRoot, `.benchmark-${label}-`))
    copyDirectory(samplesSource, cloneRoot)
    const cleanup = removeGeneratedArtifacts(cloneRoot)
    return { cloneRoot, cleanup }
}

function ensurePrepared(repoRoot, options) {
    const preparedJS = path.join(repoRoot, 'dist', 'src', 'cli', 'typegen.js')
    const preparedGo = path.join(repoRoot, 'rewrite', 'bin', 'kea-typegen-go')
    const missing = [preparedJS, preparedGo].filter((item) => !fs.existsSync(item))

    if (options.skipPrepare) {
        if (missing.length > 0) {
            throw new Error(
                `Missing prepared artifacts:\n${missing.join('\n')}\nRun ./bin/prepare-js and ./bin/prepare-go first.`,
            )
        }
        return
    }

    if (missing.length === 0) {
        return
    }
    if (missing.includes(preparedJS)) {
        run(path.join(repoRoot, 'bin', 'prepare-js'), [], { cwd: repoRoot })
    }
    if (missing.includes(preparedGo)) {
        run(path.join(repoRoot, 'bin', 'prepare-go'), [], { cwd: repoRoot })
    }
}

function buildTypegenArgs(command, samplesRoot) {
    const args = [command, '-r', samplesRoot, '-q']
    if (command === 'write') {
        args.push('--delete')
    }
    return args
}

function exactComparableText(text) {
    const normalized = text.replace(/\r\n/g, '\n')
    const lines = normalized.split('\n')
    if (lines[0] && lines[0].startsWith('// Generated by kea-typegen on ')) {
        return lines.slice(1).join('\n')
    }
    return normalized
}

function printNode(sourceFile, node) {
    return printer.printNode(ts.EmitHint.Unspecified, node, sourceFile).replace(/\s+/g, ' ').trim()
}

function canonicalPropertyName(name, sourceFile) {
    if (!name) {
        return ''
    }
    if (ts.isIdentifier(name) || ts.isPrivateIdentifier(name)) {
        return name.text
    }
    if (ts.isStringLiteralLike(name) || ts.isNumericLiteral(name)) {
        return name.text
    }
    return printNode(sourceFile, name)
}

function canonicalEntityName(name) {
    if (ts.isIdentifier(name)) {
        return name.text
    }
    return `${canonicalEntityName(name.left)}.${name.right.text}`
}

function canonicalTypeParameters(parameters, sourceFile) {
    if (!parameters || parameters.length === 0) {
        return ''
    }
    return `<${parameters.map((parameter) => canonicalTypeParameter(parameter, sourceFile)).join(', ')}>`
}

function canonicalTypeParameter(parameter, sourceFile) {
    const parts = [parameter.name.text]
    if (parameter.constraint) {
        parts.push(`extends ${canonicalType(parameter.constraint, sourceFile)}`)
    }
    if (parameter.default) {
        parts.push(`= ${canonicalType(parameter.default, sourceFile)}`)
    }
    return parts.join(' ')
}

function canonicalTupleElement(element, sourceFile) {
    if (ts.isNamedTupleMember(element)) {
        const prefix = element.dotDotDotToken ? '...' : ''
        const optional = element.questionToken ? '?' : ''
        return `${prefix}${canonicalPropertyName(element.name, sourceFile)}${optional}: ${canonicalType(element.type, sourceFile)}`
    }
    if (ts.isRestTypeNode(element)) {
        return `...${canonicalType(element.type, sourceFile)}`
    }
    if (ts.isOptionalTypeNode(element)) {
        return `${canonicalType(element.type, sourceFile)}?`
    }
    return canonicalType(element, sourceFile)
}

function canonicalStringLiteral(text) {
    return JSON.stringify(text)
}

function needsParensForArray(elementType) {
    return (
        ts.isUnionTypeNode(elementType) ||
        ts.isIntersectionTypeNode(elementType) ||
        ts.isFunctionTypeNode(elementType) ||
        ts.isConstructorTypeNode(elementType) ||
        ts.isConditionalTypeNode(elementType)
    )
}

function canonicalParameter(parameter, sourceFile) {
    const prefix = parameter.dotDotDotToken ? '...' : ''
    const optional = parameter.questionToken ? '?' : ''
    const name = printNode(sourceFile, parameter.name)
    const type = canonicalType(parameter.type, sourceFile)
    return `${prefix}${name}${optional}: ${type}`
}

function canonicalType(node, sourceFile) {
    if (!node) {
        return 'any'
    }
    if (ts.isParenthesizedTypeNode(node)) {
        return canonicalType(node.type, sourceFile)
    }
    if (ts.isUnionTypeNode(node)) {
        return node.types
            .map((item) => canonicalType(item, sourceFile))
            .sort()
            .join(' | ')
    }
    if (ts.isIntersectionTypeNode(node)) {
        return node.types
            .map((item) => canonicalType(item, sourceFile))
            .sort()
            .join(' & ')
    }
    if (ts.isArrayTypeNode(node)) {
        const element = canonicalType(node.elementType, sourceFile)
        return `${needsParensForArray(node.elementType) ? `(${element})` : element}[]`
    }
    if (ts.isTupleTypeNode(node)) {
        return `[${node.elements.map((item) => canonicalTupleElement(item, sourceFile)).join(', ')}]`
    }
    if (ts.isLiteralTypeNode(node)) {
        if (ts.isStringLiteralLike(node.literal)) {
            return canonicalStringLiteral(node.literal.text)
        }
        if (ts.isNumericLiteral(node.literal)) {
            return node.literal.text
        }
        return printNode(sourceFile, node.literal)
    }
    if (ts.isTypeReferenceNode(node)) {
        return `${canonicalEntityName(node.typeName)}${canonicalTypeArguments(node.typeArguments, sourceFile)}`
    }
    if (ts.isExpressionWithTypeArguments(node)) {
        return `${printNode(sourceFile, node.expression)}${canonicalTypeArguments(node.typeArguments, sourceFile)}`
    }
    if (ts.isFunctionTypeNode(node)) {
        return `(${node.parameters.map((parameter) => canonicalParameter(parameter, sourceFile)).join(', ')}) => ${canonicalType(node.type, sourceFile)}`
    }
    if (ts.isConstructorTypeNode(node)) {
        return `new (${node.parameters.map((parameter) => canonicalParameter(parameter, sourceFile)).join(', ')}) => ${canonicalType(node.type, sourceFile)}`
    }
    if (ts.isTypeLiteralNode(node)) {
        const members = node.members
            .map((member) => normalizeTypeElement(member, sourceFile))
            .filter(Boolean)
            .sort()
        return members.length > 0 ? `{ ${members.join('; ')} }` : '{}'
    }
    if (ts.isImportTypeNode(node)) {
        const qualifier = node.qualifier ? `.${printNode(sourceFile, node.qualifier)}` : ''
        const argument = ts.isLiteralTypeNode(node.argument)
            ? canonicalType(node.argument, sourceFile)
            : ts.isStringLiteralLike(node.argument)
              ? canonicalStringLiteral(node.argument.text)
              : printNode(sourceFile, node.argument)
        return `import(${argument})${qualifier}${canonicalTypeArguments(node.typeArguments, sourceFile)}`
    }
    if (ts.isIndexedAccessTypeNode(node)) {
        return `${canonicalType(node.objectType, sourceFile)}[${canonicalType(node.indexType, sourceFile)}]`
    }
    if (ts.isTypeOperatorNode(node)) {
        return `${ts.tokenToString(node.operator)} ${canonicalType(node.type, sourceFile)}`
    }
    if (ts.isTypeQueryNode(node)) {
        return `typeof ${printNode(sourceFile, node.exprName)}`
    }
    if (ts.isConditionalTypeNode(node)) {
        return `${canonicalType(node.checkType, sourceFile)} extends ${canonicalType(node.extendsType, sourceFile)} ? ${canonicalType(node.trueType, sourceFile)} : ${canonicalType(node.falseType, sourceFile)}`
    }
    if (ts.isInferTypeNode(node)) {
        return `infer ${canonicalTypeParameter(node.typeParameter, sourceFile)}`
    }
    if (ts.isMappedTypeNode(node)) {
        return printNode(sourceFile, node)
    }
    if (ts.isTemplateLiteralTypeNode(node)) {
        return printNode(sourceFile, node)
    }
    return printNode(sourceFile, node)
}

function canonicalTypeArguments(typeArguments, sourceFile) {
    if (!typeArguments || typeArguments.length === 0) {
        return ''
    }
    return `<${typeArguments.map((item) => canonicalType(item, sourceFile)).join(', ')}>`
}

function normalizeTypeElement(member, sourceFile) {
    if (ts.isPropertySignature(member)) {
        const name = canonicalPropertyName(member.name, sourceFile)
        if (name.startsWith('__keaTypeGenInternal')) {
            return null
        }
        const prefix = member.readonlyToken ? 'readonly ' : ''
        const optional = member.questionToken ? '?' : ''
        return `${prefix}${name}${optional}: ${canonicalType(member.type, sourceFile)}`
    }
    if (ts.isMethodSignature(member)) {
        const name = canonicalPropertyName(member.name, sourceFile)
        if (name.startsWith('__keaTypeGenInternal')) {
            return null
        }
        const optional = member.questionToken ? '?' : ''
        return `${name}${optional}${canonicalTypeParameters(member.typeParameters, sourceFile)}(${member.parameters
            .map((parameter) => canonicalParameter(parameter, sourceFile))
            .join(', ')}) => ${canonicalType(member.type, sourceFile)}`
    }
    if (ts.isCallSignatureDeclaration(member)) {
        return `call${canonicalTypeParameters(member.typeParameters, sourceFile)}(${member.parameters
            .map((parameter) => canonicalParameter(parameter, sourceFile))
            .join(', ')}) => ${canonicalType(member.type, sourceFile)}`
    }
    if (ts.isConstructSignatureDeclaration(member)) {
        return `new${canonicalTypeParameters(member.typeParameters, sourceFile)}(${member.parameters
            .map((parameter) => canonicalParameter(parameter, sourceFile))
            .join(', ')}) => ${canonicalType(member.type, sourceFile)}`
    }
    if (ts.isIndexSignatureDeclaration(member)) {
        return `[${member.parameters.map((parameter) => canonicalParameter(parameter, sourceFile)).join(', ')}]: ${canonicalType(member.type, sourceFile)}`
    }
    return printNode(sourceFile, member)
}

function normalizeImportDeclaration(statement) {
    const parts = [statement.moduleSpecifier.text]
    const clause = statement.importClause
    if (!clause) {
        return parts.join('|')
    }
    if (clause.name) {
        parts.push(`default:${clause.name.text}`)
    }
    if (clause.namedBindings) {
        if (ts.isNamespaceImport(clause.namedBindings)) {
            parts.push(`namespace:${clause.namedBindings.name.text}`)
        } else {
            const named = clause.namedBindings.elements
                .map((element) =>
                    element.propertyName ? `${element.propertyName.text} as ${element.name.text}` : element.name.text,
                )
                .sort()
            parts.push(`named:${named.join(',')}`)
        }
    }
    return parts.join('|')
}

function normalizeInterfaceDeclaration(statement, sourceFile) {
    const heritage = (statement.heritageClauses || [])
        .flatMap((clause) => clause.types.map((typeNode) => canonicalType(typeNode, sourceFile)))
        .sort()
    const members = statement.members
        .map((member) => normalizeTypeElement(member, sourceFile))
        .filter(Boolean)
        .sort()
    return JSON.stringify({
        name: statement.name.text,
        heritage,
        members,
    })
}

function normalizeTypeAliasDeclaration(statement, sourceFile) {
    return JSON.stringify({
        name: statement.name.text,
        typeParameters: (statement.typeParameters || []).map((item) => canonicalTypeParameter(item, sourceFile)),
        type: canonicalType(statement.type, sourceFile),
    })
}

function normalizeGeneratedFile(text, fileName) {
    const sourceFile = ts.createSourceFile(fileName, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
    const normalized = {
        imports: [],
        interfaces: [],
        typeAliases: [],
        otherStatements: [],
    }

    for (const statement of sourceFile.statements) {
        if (ts.isImportDeclaration(statement)) {
            normalized.imports.push(normalizeImportDeclaration(statement))
        } else if (ts.isInterfaceDeclaration(statement)) {
            normalized.interfaces.push(normalizeInterfaceDeclaration(statement, sourceFile))
        } else if (ts.isTypeAliasDeclaration(statement)) {
            normalized.typeAliases.push(normalizeTypeAliasDeclaration(statement, sourceFile))
        } else if (!ts.isEmptyStatement(statement)) {
            normalized.otherStatements.push(printNode(sourceFile, statement))
        }
    }

    normalized.imports.sort()
    normalized.interfaces.sort()
    normalized.typeAliases.sort()
    normalized.otherStatements.sort()

    return JSON.stringify(normalized)
}

function compareOutputs(tsDir, goDir) {
    const tsFiles = new Map(listGeneratedTypeFiles(tsDir).map((file) => [file, path.join(tsDir, file)]))
    const goFiles = new Map(listGeneratedTypeFiles(goDir).map((file) => [file, path.join(goDir, file)]))
    const allFiles = [...new Set([...tsFiles.keys(), ...goFiles.keys()])].sort()

    const summary = {
        totalFiles: allFiles.length,
        comparableFiles: 0,
        exactMatches: 0,
        semanticMatches: 0,
        tsOnly: [],
        goOnly: [],
        semanticDiffs: [],
    }

    for (const file of allFiles) {
        const tsPath = tsFiles.get(file)
        const goPath = goFiles.get(file)

        if (tsPath && !goPath) {
            summary.tsOnly.push(file)
            continue
        }
        if (!tsPath && goPath) {
            summary.goOnly.push(file)
            continue
        }

        summary.comparableFiles += 1
        const tsText = fs.readFileSync(tsPath, 'utf8')
        const goText = fs.readFileSync(goPath, 'utf8')

        if (exactComparableText(tsText) === exactComparableText(goText)) {
            summary.exactMatches += 1
        }

        if (normalizeGeneratedFile(tsText, file) === normalizeGeneratedFile(goText, file)) {
            summary.semanticMatches += 1
        } else {
            summary.semanticDiffs.push(file)
        }
    }

    summary.semanticAccuracy = summary.totalFiles === 0 ? 100 : (summary.semanticMatches / summary.totalFiles) * 100
    summary.exactAccuracy = summary.totalFiles === 0 ? 100 : (summary.exactMatches / summary.totalFiles) * 100
    return summary
}

function main() {
    const argv = process.argv.slice(2)
    if (argv.length === 0) {
        printHelp()
        process.exit(1)
    }

    const options = parseArgs(argv)
    const repoRoot = path.resolve(__dirname, '..')
    const samplesSource = options.samples || path.join(repoRoot, 'samples')
    const workspaceRoot = fs.mkdtempSync(path.join(repoRoot, '.benchmark-work-'))
    const jsCLI = path.join(repoRoot, 'bin', 'kea-typegen-js')
    const goCLI = path.join(repoRoot, 'bin', 'kea-typegen-go')
    const benchmarkPaths = []
    const cleanupTargets = new Set([workspaceRoot])
    let cleaned = false

    const cleanup = () => {
        if (cleaned || options.keepWorkspace) {
            return
        }
        cleaned = true
        for (const target of [...cleanupTargets].sort((a, b) => b.length - a.length)) {
            fs.rmSync(target, { recursive: true, force: true })
        }
    }

    const handleSignal = (signal) => {
        cleanup()
        process.stderr.write(`\n${signal}\n`)
        process.exit(130)
    }

    process.on('SIGINT', () => handleSignal('SIGINT'))
    process.on('SIGTERM', () => handleSignal('SIGTERM'))

    try {
        ensurePrepared(repoRoot, options)

        const jsRunner = (suffix) => {
            const { cloneRoot } = createBenchmarkClone(repoRoot, samplesSource, `js-${suffix}`)
            benchmarkPaths.push(cloneRoot)
            cleanupTargets.add(cloneRoot)
            const duration = timedRun(jsCLI, buildTypegenArgs(options.command, cloneRoot), {
                cwd: repoRoot,
                expectedStatuses: options.command === 'check' ? [0, 1] : [0],
            })
            if (!options.keepWorkspace) {
                fs.rmSync(cloneRoot, { recursive: true, force: true })
                cleanupTargets.delete(cloneRoot)
            }
            return duration
        }

        const goRunner = (suffix) => {
            const { cloneRoot } = createBenchmarkClone(repoRoot, samplesSource, `go-${suffix}`)
            benchmarkPaths.push(cloneRoot)
            cleanupTargets.add(cloneRoot)
            const duration = timedRun(goCLI, buildTypegenArgs(options.command, cloneRoot), {
                cwd: repoRoot,
                expectedStatuses: options.command === 'check' ? [0, 1] : [0],
            })
            if (!options.keepWorkspace) {
                fs.rmSync(cloneRoot, { recursive: true, force: true })
                cleanupTargets.delete(cloneRoot)
            }
            return duration
        }

        const tsStats = benchmarkImplementation('TypeScript', options.iterations, options.warmup, jsRunner)
        const goStats = benchmarkImplementation('Go', options.iterations, options.warmup, goRunner)

        let accuracy = null
        let cleanupSummary = null
        if (options.command === 'write') {
            const jsAccuracyClone = createBenchmarkClone(repoRoot, samplesSource, 'accuracy-js')
            const goAccuracyClone = createBenchmarkClone(repoRoot, samplesSource, 'accuracy-go')
            benchmarkPaths.push(jsAccuracyClone.cloneRoot, goAccuracyClone.cloneRoot)
            cleanupTargets.add(jsAccuracyClone.cloneRoot)
            cleanupTargets.add(goAccuracyClone.cloneRoot)
            cleanupSummary = jsAccuracyClone.cleanup

            run(jsCLI, buildTypegenArgs('write', jsAccuracyClone.cloneRoot), { cwd: repoRoot })
            run(goCLI, buildTypegenArgs('write', goAccuracyClone.cloneRoot), { cwd: repoRoot })

            accuracy = compareOutputs(jsAccuracyClone.cloneRoot, goAccuracyClone.cloneRoot)

            if (!options.keepWorkspace) {
                fs.rmSync(jsAccuracyClone.cloneRoot, { recursive: true, force: true })
                fs.rmSync(goAccuracyClone.cloneRoot, { recursive: true, force: true })
                cleanupTargets.delete(jsAccuracyClone.cloneRoot)
                cleanupTargets.delete(goAccuracyClone.cloneRoot)
            }
        }

        const speedup = tsStats.mean / goStats.mean
        const faster = speedup > 1
        const timeDeltaPercent = faster
            ? (1 - goStats.mean / tsStats.mean) * 100
            : (goStats.mean / tsStats.mean - 1) * 100

        const report = {
            generatedAt: new Date().toISOString(),
            repoRoot,
            workspaceRoot,
            samplesSource,
            options,
            cleanupSummary,
            benchmarkPaths: options.keepWorkspace ? benchmarkPaths : [],
            performance: {
                typeScript: tsStats,
                go: goStats,
                speedup,
                faster,
                timeDeltaPercent,
            },
            accuracy,
        }

        const reportPath = path.join(workspaceRoot, 'benchmark-report.json')
        fs.writeFileSync(reportPath, JSON.stringify(report, null, 2))

        console.log(`Benchmark command: ${options.command}`)
        console.log(`Samples source: ${samplesSource}`)
        if (cleanupSummary) {
            console.log(
                `Clean baseline: removed ${cleanupSummary.removedTypeFiles} generated type files and ${cleanupSummary.removedCacheDirs} cache directories per clone`,
            )
        }
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
            console.log(
                `Go is ${speedup.toFixed(2)}x faster on mean runtime (${timeDeltaPercent.toFixed(1)}% less time).`,
            )
        } else {
            console.log(
                `Go is ${(1 / speedup).toFixed(2)}x slower on mean runtime (${timeDeltaPercent.toFixed(1)}% more time).`,
            )
        }

        if (accuracy) {
            console.log('')
            console.log(
                `Semantic accuracy: ${accuracy.semanticMatches}/${accuracy.totalFiles} (${formatPercent(accuracy.semanticAccuracy)})`,
            )
            console.log(
                `Exact accuracy: ${accuracy.exactMatches}/${accuracy.totalFiles} (${formatPercent(accuracy.exactAccuracy)})`,
            )
            console.log(`TS_ONLY: ${accuracy.tsOnly.length}`)
            console.log(`GO_ONLY: ${accuracy.goOnly.length}`)
            console.log(`Semantic diffs: ${accuracy.semanticDiffs.length}`)

            const noteworthy = [
                ...accuracy.tsOnly.map((file) => `TS_ONLY ${file}`),
                ...accuracy.goOnly.map((file) => `GO_ONLY ${file}`),
                ...accuracy.semanticDiffs.map((file) => `DIFF ${file}`),
            ].slice(0, 10)
            if (noteworthy.length > 0) {
                console.log(`Top differences: ${noteworthy.join(', ')}`)
            }
        }

        if (!options.keepWorkspace) {
            cleanup()
            console.log('Benchmark workspace removed (--cleanup).')
        } else {
            console.log(`Benchmark report: ${reportPath}`)
            console.log(`Benchmark workspace: ${workspaceRoot}`)
        }
    } catch (error) {
        cleanup()
        throw error
    }
}

try {
    main()
} catch (error) {
    console.error(error.message || String(error))
    process.exit(1)
}
