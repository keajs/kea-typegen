import { spawn } from 'child_process'
import * as path from 'path'
import * as ts from 'typescript'
import { openNativeProgram } from './native'
import { nodeToString } from './print/print'
import { OutputProgram } from './program'
import { AppOptions, ParsedLogic, PluginModule, TypeBuilderModule, VisitKeaPropertyArguments } from './types'
import { cloneNode } from 'ts-clone-node'
import { getLogicPathString, relocateImportTypes } from './utils'
import { deleteOrphanedTypes, visitBuiltin, visitKeaCalls, visitResetContext } from './visit/visit'

interface Step {
    Name: string
    Index: number
}
interface Logic {
    Name: string
    File: string
    End: number
    ImportedFrom: string
    TypeArgs: string[]
    BuilderArray: boolean
    HasPath: boolean
    Path: string[]
    Steps: Step[]
    Declaration: string
    Values: string[]
    SharedListeners: boolean
    Annotations: { End: number; Type: string; SourceValue: string }[]
    Imports: { File: string; Name: string }[]
}
interface Project {
    Files: string[]
    ESM: string[]
    Types: string[]
    Logics: Logic[]
    PluginFiles: string[]
    PluginContexts: string[]
    CustomGlobalHooks: boolean
    Diagnostics: string
}

async function analyze(options: AppOptions): Promise<Project> {
    const parent = path.resolve(__dirname, '..')
    const root = ts.sys.fileExists(path.join(parent, 'go/go.mod')) ? parent : path.dirname(parent)
    const platform = process.platform + '-' + process.arch
    const executable = 'kea-typegen-go' + (process.platform === 'win32' ? '.exe' : '')
    let binary = path.join(root, 'go', 'bin', platform, executable)
    if (!ts.sys.fileExists(binary)) {
        const pkg = '@keajs/kea-typegen-' + platform
        try {
            binary = path.join(path.dirname(require.resolve(pkg + '/package.json', { paths: [root] })), executable)
        } catch (error) {
            if (error.code !== 'MODULE_NOT_FOUND') throw error
            throw new Error(
                `Missing ${pkg}. Install kea-typegen with optional dependencies or run npm run build:go in ${root}.`,
            )
        }
    }
    return new Promise((resolve, reject) => {
        const child = spawn(binary, [], { stdio: ['pipe', 'pipe', 'pipe'] })
        const data: Buffer[] = []
        const errors: Buffer[] = []
        child.stdout.on('data', (chunk) => data.push(chunk))
        child.stderr.on('data', (chunk) => errors.push(chunk))
        child.on('error', (err: NodeJS.ErrnoException) =>
            reject(
                err.code === 'ENOENT'
                    ? new Error(`Missing Go engine at ${binary}. Run npm run build:go in ${root}.`)
                    : err,
            ),
        )
        child.stdin.on('error', (err) => {
            if ((err as NodeJS.ErrnoException).code !== 'EPIPE') reject(err)
        })
        child.on('close', (code, signal) => {
            if (code !== 0)
                return reject(new Error(`Go typegen exited with ${signal || code}: ${Buffer.concat(errors)}`))
            try {
                resolve(JSON.parse(Buffer.concat(data).toString()))
            } catch (err) {
                reject(err)
            }
        })
        child.stdin.end(JSON.stringify(options))
    })
}

// Source edits use classic AST nodes with UTF-16 positions. Only materialize the
// call and parameter nodes the worker actually referenced.
function atEnd<T extends ts.Node>(root: ts.Node, end: number, predicate: (n: ts.Node) => n is T): T {
    function find(n: ts.Node): T | undefined {
        if (n.end < end || n.pos >= end) return
        if (n.end === end && predicate(n)) return n
        return ts.forEachChild(n, find)
    }
    const node = find(root)
    if (!node) throw new Error(`Cannot locate generated type edit in ${root.getSourceFile().fileName} at ${end}`)
    return node
}
function emptyLogic(g: Logic, node: ts.Node, options: AppOptions): ParsedLogic {
    const base = g.File.slice(0, -path.extname(g.File).length) + 'Type.ts'
    const typeFileName = path.resolve(options.typesPath ?? options.rootPath, path.relative(options.rootPath, base))
    const pathString = getLogicPathString(options, g.File)
    return {
        logicName: g.Name,
        logicTypeName: g.Name + 'Type',
        fileName: g.File,
        typeFileName,
        node,
        inputBuilderArray: g.BuilderArray,
        logicTypeArguments: g.TypeArgs || [],
        logicTypeImported: !!g.ImportedFrom && path.resolve(g.ImportedFrom) === typeFileName,
        checker: undefined,
        pathString,
        path: pathString.split('.'),
        hasPathInLogic: false,
        hasKeyInLogic: false,
        actions: [],
        reducers: [],
        selectors: [],
        listeners: [],
        sharedListeners: [],
        events: {},
        extraActions: {},
        extraInput: {},
        extraLogicFields: {},
        selectorParamAnnotations: [],
        typeReferencesToImportFromFiles: {},
        importFromKeaInLogicType: new Set(),
    }
}

function hydrate(g: Logic, logic: ParsedLogic) {
    const source = ts.createSourceFile('declaration.ts', g.Declaration, ts.ScriptTarget.Latest, true)
    const diagnostics = (source as ts.SourceFile & { parseDiagnostics: readonly ts.Diagnostic[] }).parseDiagnostics
    if (diagnostics.length) {
        throw new Error(
            `Invalid declaration for ${g.Name} in ${g.File}: ${ts.flattenDiagnosticMessageText(diagnostics[0].messageText, '\n')}`,
        )
    }
    const declarations = source.statements.map((statement) => {
        let node = cloneNode(statement, { typescript: ts })
        if (ts.isInterfaceDeclaration(node) && node.name.text === logic.logicTypeName) {
            node = relocateImportTypes(node, g.File, logic.typeFileName)
        }
        return nodeToString(node)
    })
    logic.generated = {
        declaration: declarations.join('\n\n'),
        values: new Set(g.Values),
        sharedListeners: g.SharedListeners,
    }
    logic.path = g.Path ?? []
    logic.pathString = logic.path.join('.')
    logic.hasPathInLogic = g.HasPath
    for (const i of g.Imports || []) (logic.typeReferencesToImportFromFiles[i.File] ??= new Set()).add(i.Name)
    logic.selectorParamAnnotations = (g.Annotations || []).map((a) => {
        const source = ts.createSourceFile('annotation.ts', 'type T = ' + a.Type, ts.ScriptTarget.Latest, true)
        return {
            parameter: atEnd(logic.node.parent, a.End, ts.isParameter),
            typeNode: cloneNode((source.statements[0] as ts.TypeAliasDeclaration).type, { typescript: ts }),
            sourceValue: a.SourceValue || undefined,
        }
    })
}

// Global JS hooks can inspect every builder and mutate its partially-built logic.
// Materialize that state on first access, replaying built-ins only. Each hook
// still runs exactly once, in declaration order. Name-only dispatch stays cheap.
function applyPlugins(
    g: Logic,
    logic: ParsedLogic,
    plugins: PluginModule[],
    options: AppOptions,
    openPluginSource: () => { node: ts.Node; checker: ts.TypeChecker },
): boolean {
    let cursor = 0
    let args: VisitKeaPropertyArguments
    const materialize = (index: number) => {
        if (!logic.checker) Object.assign(logic, openPluginSource())
        const input = (logic.node.parent as ts.CallExpression).arguments[0]
        while (cursor <= index) {
            const { Name, Index } = g.Steps[cursor++]
            const expression = ts.isArrayLiteralExpression(input)
                ? (input.elements[Index] as ts.CallExpression).arguments[0]
                : ((input as ts.ObjectLiteralExpression).properties[Index] as ts.PropertyAssignment).initializer
            const property = ts.isPropertyAssignment(expression.parent)
                ? logic.checker.getSymbolAtLocation(expression.parent.name)
                : undefined
            const type = property
                ? logic.checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration)
                : logic.checker.getTypeAtLocation(expression)
            args = visitBuiltin(logic, options, Name, expression, type)
        }
        return args
    }
    for (const [index, step] of (g.Steps || []).entries()) {
        let current = cursor ? materialize(index) : undefined
        const real = () => (current ??= materialize(index))
        const lazy: VisitKeaPropertyArguments = {
            name: step.Name,
            appOptions: options,
            get type() {
                return real().type
            },
            get typeNode() {
                return real().typeNode
            },
            get parsedLogic() {
                return real().parsedLogic
            },
            get node() {
                return real().node
            },
            get checker() {
                return real().checker
            },
            get gatherImports() {
                return real().gatherImports
            },
            get cloneNode() {
                return real().cloneNode
            },
            get prepareForPrint() {
                return real().prepareForPrint
            },
            get getTypeNodeForNode() {
                return real().getTypeNodeForNode
            },
        }
        for (const plugin of plugins) plugin.visitKeaProperty(current ?? lazy)
    }
    return cursor > 0
}

export async function openGoProgram(options: AppOptions) {
    const rootPath = path.resolve(options.rootPath || '.')
    options = {
        ...options,
        rootPath,
        typesPath: path.resolve(options.typesPath || rootPath),
        tsConfigPath: options.tsConfigPath && path.resolve(options.tsConfigPath),
        sourceFilePath: options.sourceFilePath && path.resolve(options.sourceFilePath),
    }
    const g = await analyze(options)
    if (g.Diagnostics) options.log(g.Diagnostics)
    const names = g.Files || []
    const files = new Set(names)
    const esm = new Set(g.ESM)
    const sourceFiles = new Map<string, ts.SourceFile>()
    const program: OutputProgram = {
        getSourceFileNames: () => names,
        readSourceText: ts.sys.readFile,
        getCompilerOptions: () => ({ types: g.Types ?? undefined }),
        requiresImportExtension: (name, fallback) => esm.has(files.has(name) ? name : fallback),
        getSourceFile(name) {
            if (!sourceFiles.has(name)) {
                const text = ts.sys.readFile(name)
                if (text === undefined) throw new Error(`Missing TypeScript source file: ${name}`)
                sourceFiles.set(name, ts.createSourceFile(name, text, ts.ScriptTarget.Latest, true))
            }
            return sourceFiles.get(name)
        },
    }
    const compat = { ...options, compiler: 'native' as const, delete: false, showTsErrors: false }
    const session = g.PluginFiles?.length || g.PluginContexts?.length ? await openNativeProgram(compat) : undefined
    try {
        const plugins: PluginModule[] = []
        const parsed: ParsedLogic[] = []
        if (g.PluginContexts?.length) {
            for (const name of g.PluginContexts) {
                const source = session.program.getSourceFile(name)
                ts.forEachChild(source, visitResetContext(session.program.getTypeChecker(source), plugins, compat))
            }
        }
        const byFile = new Map<string, Logic[]>()
        for (const record of g.Logics || []) {
            const group = byFile.get(record.File)
            if (group) group.push(record)
            else byFile.set(record.File, [record])
        }
        const pluginFiles = new Set(g.PluginFiles)
        const builders: TypeBuilderModule[] = []
        let goCount = 0
        for (const name of names) {
            if (pluginFiles.has(name)) {
                const source = session.program.getSourceFile(name)
                sourceFiles.set(name, source)
                ts.forEachChild(
                    source,
                    visitKeaCalls(session.program.getTypeChecker(source), parsed, source, compat, plugins, builders),
                )
            } else {
                for (const record of byFile.get(name) || []) {
                    const source = program.getSourceFile(name)
                    const node = atEnd(source, record.End, ts.isIdentifier)
                    const logic = emptyLogic(record, node, options)
                    if (
                        !g.CustomGlobalHooks ||
                        !plugins.length ||
                        !applyPlugins(record, logic, plugins, compat, () => {
                            const file = session.program.getSourceFile(name)
                            sourceFiles.set(name, file)
                            return {
                                node: atEnd(file, record.End, ts.isIdentifier),
                                checker: session.program.getTypeChecker(file),
                            }
                        })
                    ) {
                        hydrate(record, logic)
                        goCount++
                    }
                    parsed.push(logic)
                }
            }
        }
        deleteOrphanedTypes(names, options)
        if (options.verbose)
            options.log(
                `Go generated ${goCount} logic types. JavaScript plugin compatibility generated ${parsed.length - goCount}.`,
            )
        return { program, parsed }
    } finally {
        session?.close()
    }
}
