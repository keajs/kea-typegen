// The checker lives in the native process. The visitors, printers and existing Kea
// plugins share classic syntax nodes. Only files touched by those consumers are
// parsed in JS. No classic Program or language service is created here.
import * as ts from 'typescript'
import * as path from 'path'
import type * as native from 'typescript-native/sync' with { 'resolution-mode': 'import' }
import type * as ast from 'typescript-native/ast' with { 'resolution-mode': 'import' }
import { LogicProgram } from './program'
import { AppOptions } from './types'

type SyntaxNode = ts.Node | ast.Node

// Both public ASTs use UTF-16 source ranges. Several nested nodes can span the
// same text (e.g. an identifier and a type reference), so include the syntax kind.
function correspondingNode<N extends SyntaxNode>(root: N, source: SyntaxNode, kind: number): N {
    const find = (node: N): N | undefined => {
        if (node.pos > source.pos || node.end < source.end) return undefined
        if (node.pos === source.pos && node.end === source.end && node.kind === kind) return node
        return node.forEachChild((child) => find(child as N))
    }
    const match = find(root)
    if (!match) {
        throw new Error(
            `Cannot map TypeScript syntax at ${root.getSourceFile().fileName} [${source.pos}, ${source.end})`,
        )
    }
    return match
}

export async function openNativeProgram(options: AppOptions) {
    const {
        API,
        ModuleKind,
        ModuleResolutionKind,
        SymbolFlags,
        TypeFlags,
        TypeFormatFlags,
        formatDiagnosticsWithColorAndContext,
    } = await import('typescript-native/sync')
    const { SyntaxKind } = await import('typescript-native/ast')
    const api = new API()
    try {
        const config = options.tsConfigPath && api.parseConfigFile(path.resolve(options.tsConfigPath))
        const errors = config?.errors?.filter((error) => !(options.sourceFilePath && error.code === 18003))
        if (errors?.length) {
            throw new Error(formatDiagnosticsWithColorAndContext(errors, api))
        }
        const files = options.sourceFilePath ? [path.resolve(options.sourceFilePath)] : config.fileNames
        const projectOptions = { ...config?.options, noEmit: true }
        if (options.sourceFilePath && /\.[cm]?jsx?$/i.test(options.sourceFilePath)) projectOptions.allowJs = true
        const remote = api.createProgram(files, projectOptions, {
            projectReferences: config?.projectReferences,
        })
        if (options.sourceFilePath && !remote.getSourceFile(path.resolve(options.sourceFilePath))) {
            throw new Error(`Source file is missing from the TypeScript program: ${options.sourceFilePath}`)
        }
        const checker = remote.getProject().checker
        if (options.showTsErrors) {
            const errors = [
                ...remote.getProgramDiagnostics(),
                ...remote.getSyntacticDiagnostics(),
                ...remote.getSemanticDiagnostics(),
            ]
            if (errors.length) options.log(formatDiagnosticsWithColorAndContext(errors, api))
        }
        const names = remote.getSourceFileNames()
        const sources = new Map<string, ts.SourceFile>()
        function getSourceFile(name: string) {
            let source = sources.get(name)
            if (!source) {
                const file = remote.getSourceFile(name)
                if (!file) return undefined
                source = ts.createSourceFile(file.fileName, file.text, ts.ScriptTarget.Latest, true)
                sources.set(name, source)
            }
            return source
        }

        const toRemote = new WeakMap<ts.Node, ast.Node>()
        const toLocal = new WeakMap<ast.Node, ts.Node>()
        function remoteNode(node: ts.Node) {
            if (!node) return undefined
            let match = toRemote.get(node)
            if (!match) {
                const root = remote.getSourceFile(node.getSourceFile().fileName)
                match = correspondingNode<ast.Node>(root, node, SyntaxKind[ts.SyntaxKind[node.kind]])
                toRemote.set(node, match)
                toLocal.set(match, node)
            }
            return match
        }
        function localDeclaration(handle?: native.NodeHandle<ast.Declaration>) {
            if (!handle) return undefined
            const node = handle.resolve()
            let match = toLocal.get(node)
            if (!match) {
                const root = getSourceFile(node.getSourceFile().fileName)
                match = correspondingNode<ts.Node>(root, node, ts.SyntaxKind[SyntaxKind[node.kind]])
                toLocal.set(node, match)
                toRemote.set(match, node)
            }
            return match as ts.Declaration
        }

        function symbol(raw?: native.Symbol) {
            if (!raw) return undefined
            return {
                raw,
                name: raw.name,
                get isDefaultLibrarySymbol() {
                    const first = raw.declarations[0]
                    return first && remote.getSourceFileMetadataByPath(first.path)?.isDefaultLibrary
                },
                flags: raw.flags,
                escapedName: raw.escapedName,
                getName: () => raw.name,
                getDeclarations: () => raw.declarations.map(localDeclaration),
                get declarations() {
                    return this.getDeclarations()
                },
                get valueDeclaration() {
                    return localDeclaration(raw.valueDeclaration)
                },
            }
        }
        type SymbolView = ReturnType<typeof symbol>
        // Some elementary TypeFlags moved in 7.1. Translate individual bits before
        // exposing them to a plugin that compares against its classic TS enums.
        const typeFlagBits = Object.entries(ts.TypeFlags)
            .filter(([name, bit]) => typeof bit === 'number' && bit > 0 && (bit & (bit - 1)) === 0 && TypeFlags[name])
            .map(([name, bit]) => [TypeFlags[name] as number, bit as number])
        function type(raw: native.Type) {
            return {
                raw,
                get flags() {
                    return typeFlagBits.reduce((flags, [from, to]) => (raw.flags & from ? flags | to : flags), 0)
                },
                get symbol() {
                    return symbol(raw.getSymbol())
                },
                get aliasSymbol() {
                    return symbol(raw.getAliasSymbol())
                },
                getProperties: () => raw.getProperties().map(symbol),
                getProperty: (name: string) => symbol(raw.getProperty(name)),
                getCallSignatures: () => raw.getCallSignatures().map(signature),
            }
        }
        function signature(raw: native.Signature) {
            return raw && { raw, getReturnType: () => type(raw.getReturnType()) }
        }
        type TypeView = ReturnType<typeof type>
        type SignatureView = ReturnType<typeof signature>

        function checkerForFile(file: ts.SourceFile) {
            const scope = remote.getSourceFile(file.fileName)
            const inferredTypes = new WeakMap<ts.SourceFile, ast.Node>()
            function parseType(text: string, location: ast.Node) {
                const file = ts.createSourceFile('inferred.ts', 'type T = ' + text, ts.ScriptTarget.Latest, true)
                inferredTypes.set(file, location)
                return (file.statements[0] as ts.TypeAliasDeclaration).type
            }
            const view = {
                getTypeAtLocation: (n: ts.Node) => type(checker.getTypeAtLocation(remoteNode(n))),
                getTypeFromTypeNode: (n: ts.TypeNode) =>
                    type(checker.getTypeFromTypeNode(remoteNode(n) as ast.TypeNode)),
                getTypeOfSymbolAtLocation: (s: SymbolView, n: ts.Node) =>
                    type(checker.getTypeOfSymbolAtLocation(s.raw, remoteNode(n) ?? scope)),
                getSymbolAtLocation(n: ts.Node) {
                    const location = n.parent && inferredTypes.get(n.getSourceFile())
                    if (location) {
                        let name = n
                        while (ts.isQualifiedName(name)) name = name.left
                        return ts.isIdentifier(name)
                            ? symbol(
                                  checker.resolveName(
                                      name.text,
                                      SymbolFlags.Type | SymbolFlags.Namespace | SymbolFlags.Alias,
                                      location,
                                  ),
                              )
                            : undefined
                    }
                    if (!n.parent) return undefined
                    return symbol(checker.getSymbolAtLocation(remoteNode(n)))
                },
                getExportsOfModule: (s: SymbolView) => checker.getExportsOfModule(s.raw).map(symbol),
                getPropertiesOfType: (t: TypeView) => t.getProperties(),
                getSignatureFromDeclaration: (n: ts.SignatureDeclaration) =>
                    signature(checker.getSignatureFromDeclaration(remoteNode(n) as ast.Declaration)),
                getReturnTypeOfSignature: (s: SignatureView) => s.getReturnType(),
                typeToTypeNode(t: TypeView, enclosing?: ts.Node, flags?: number) {
                    const location = remoteNode(enclosing) ?? scope
                    // typeToString combines the builder and printer in one native request.
                    if (flags === ts.NodeBuilderFlags.NoTruncation) {
                        return parseType(checker.typeToString(t.raw, location, TypeFormatFlags.NoTruncation), location)
                    }
                    const node = checker.typeToTypeNode(t.raw, location, flags)
                    return node && parseType(api.printer.printNode(node), location)
                },
                signatureToSignatureDeclaration(
                    s: SignatureView,
                    kind: ts.SyntaxKind,
                    enclosing?: ts.Node,
                    flags?: number,
                ) {
                    const location = remoteNode(enclosing) ?? scope
                    const node = checker.signatureToSignatureDeclaration(
                        s.raw,
                        SyntaxKind[ts.SyntaxKind[kind]],
                        location,
                        flags,
                    )
                    return node && parseType(api.printer.printNode(node), location)
                },
            }
            // Legacy plugins receive the same syntax model as the built-in visitors.
            // Plugins using other classic checker methods can select the classic engine.
            return view as unknown as ts.TypeChecker
        }
        const compilerOptions = remote.getCompilerOptions()
        const nodeResolution =
            compilerOptions.moduleResolution === undefined
                ? [ModuleKind.Node16, ModuleKind.Node18, ModuleKind.Node20, ModuleKind.NodeNext].includes(
                      compilerOptions.module,
                  )
                : [ModuleResolutionKind.Node16, ModuleResolutionKind.NodeNext].includes(
                      compilerOptions.moduleResolution,
                  )
        const program: LogicProgram = {
            requiresImportExtension(name, fallback) {
                if (!nodeResolution) return false
                const metadata =
                    remote.getSourceFileMetadata(name) ?? (fallback && remote.getSourceFileMetadata(fallback))
                return metadata?.impliedNodeFormat === ModuleKind.ESNext
            },
            getSourceFileNames: () => names,
            readSourceText: ts.sys.readFile,
            getSourceFile,
            getTypeChecker: checkerForFile,
            getCompilerOptions: () => ({ types: remote.getCompilerOptions().types }),
        }
        return {
            program,
            close() {
                try {
                    remote.dispose()
                } finally {
                    api.close()
                }
            },
        }
    } catch (error) {
        api.close()
        throw error
    }
}
