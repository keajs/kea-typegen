import * as ts from 'typescript'
import * as path from 'path'
import { cloneNode } from 'ts-clone-node'
import { visitProgram } from './visit/visit'
import { parsedLogicToTypeString } from './print/print'
import { AppOptions, NameType, ParsedLogic } from './types'
import { factory, isSourceFile, NodeBuilderFlags, SyntaxKind } from 'typescript'

export function logicSourceToLogicType(logicSource: string, appOptions?: AppOptions) {
    const program = programFromSource(logicSource)
    const [parsedLogic] = visitProgram(program)
    return parsedLogicToTypeString(parsedLogic, appOptions)
}

export function sourceToSourceFile(sourceCode: string, filename: string = 'logic.ts') {
    return ts.createSourceFile(filename, sourceCode, ts.ScriptTarget.ES5, true)
}

export function programFromSource(sourceCode: string) {
    const options = {}
    const compilerHost = ts.createCompilerHost(options)
    compilerHost.getSourceFile = (fileName) => (fileName === 'logic.ts' ? sourceToSourceFile(sourceCode) : undefined)
    return ts.createProgram(['logic.ts'], options, compilerHost)
}

function rejectImportPath(path: string): boolean {
    if (path.includes('/node_modules/typescript/') || path === 'typescript' || path.startsWith('typescript/')) {
        return true
    }
    return false
}

export function isKeaCall(node: ts.Node, checker: ts.TypeChecker) {
    if (!ts.isIdentifier(node)) {
        return false
    }

    if (!node.parent || !ts.isCallExpression(node.parent)) {
        return false
    }

    const symbol = checker.getSymbolAtLocation(node)
    if (!symbol || symbol.getName() !== 'kea') {
        return false
    }

    const input = node.parent.arguments[0]

    if (!input) {
        return false
    }

    return ts.isObjectLiteralExpression(input) || ts.isArrayLiteralExpression(input)
}

export function getTypeNodeForNode(node: ts.Node, checker: ts.TypeChecker): ts.TypeNode {
    let typeNode
    if (node) {
        if (ts.isAsExpression(node)) {
            typeNode = node.type
            if (ts.isParenthesizedTypeNode(typeNode)) {
                typeNode = typeNode.type
            }
        } else if (node?.kind === SyntaxKind.TrueKeyword || node?.kind === SyntaxKind.FalseKeyword) {
            typeNode = factory.createKeywordTypeNode(SyntaxKind.BooleanKeyword)
        } else if (ts.isStringLiteralLike(node)) {
            typeNode = factory.createKeywordTypeNode(SyntaxKind.StringKeyword)
        } else if (ts.isNumericLiteral(node)) {
            typeNode = factory.createKeywordTypeNode(SyntaxKind.NumberKeyword)
        } else if (ts.isArrayLiteralExpression(node) && node.elements.length === 0) {
            typeNode = factory.createArrayTypeNode(factory.createKeywordTypeNode(SyntaxKind.AnyKeyword))
        } else {
            typeNode = checker.typeToTypeNode(checker.getTypeAtLocation(node), undefined, NodeBuilderFlags.NoTruncation)
        }
    } else {
        typeNode = factory.createKeywordTypeNode(SyntaxKind.AnyKeyword)
    }
    return typeNode
}

export function getAndGatherTypeNodeForDefaultValue(
    defaultValue: ts.Node,
    checker: ts.TypeChecker,
    parsedLogic: ParsedLogic,
): ts.TypeNode {
    const typeNode = getTypeNodeForNode(defaultValue, checker)
    gatherImports(typeNode, checker, parsedLogic)
    return cloneNodeSorted(typeNode)
}

export function getParameterDeclaration(param: ts.ParameterDeclaration) {
    return factory.createParameterDeclaration(
        undefined,
        undefined,
        factory.createIdentifier(param.name.getText()),
        param.initializer || param.questionToken ? factory.createToken(SyntaxKind.QuestionToken) : undefined,
        cloneNodeSorted(param.type || factory.createKeywordTypeNode(SyntaxKind.AnyKeyword)),
        undefined,
    )
}

export const toSpaces = (key) => key.replace(/(?:^|\.?)([A-Z])/g, (x, y) => ' ' + y.toLowerCase()).replace(/^ /, '')

export function getActionTypeCreator(parsedLogic: ParsedLogic) {
    return function (actionName) {
        return `${toSpaces(actionName)} (${parsedLogic.pathString})`
    }
}

export function cleanDuplicateAnyNodes(reducers: NameType[]): NameType[] {
    let newReducers = {}

    for (const reducer of reducers) {
        // if (reducer.name === 'featureFlag') {
        //     debugger
        // }
        if (!newReducers[reducer.name] || !isAnyUnknown(reducer.typeNode)) {
            newReducers[reducer.name] = reducer
        }
    }

    return Object.values(newReducers)
}

export function extractImportedActions(
    actionObjects: ts.Expression | ts.ObjectLiteralExpression,
    checker: ts.TypeChecker,
    parsedLogic: ParsedLogic,
) {
    let extraActions = {}

    if (ts.isObjectLiteralExpression(actionObjects)) {
        // actionObjects =  { [githubLogic.actionTypes.setRepositories]: () => ... }
        for (const property of actionObjects.properties) {
            // property.name ==> [githubLogic.actionTypes.setRepositories]
            if (ts.isComputedPropertyName(property.name)) {
                let propertyExpression = property.name.expression

                if (ts.isPropertyAccessExpression(propertyExpression)) {
                    const { name, expression } = propertyExpression
                    const actionName = name.escapedText

                    const nameSymbol = checker.getSymbolAtLocation(property.name)
                    let actionType = nameSymbol.escapedName as string

                    if (ts.isPropertyAccessExpression(expression)) {
                        // expression.expression ==> githubLogic.actionTypes
                        // expression.name ==> setRepositories

                        const logicForAction = ts.isCallExpression(expression.expression)
                            ? expression.expression.expression
                            : expression.expression
                        const symbol = checker.getSymbolAtLocation(logicForAction)
                        const symbolType = checker.getTypeOfSymbolAtLocation(symbol, logicForAction)

                        const actionCreatorsProperty = symbolType
                            .getProperties()
                            .find((p) => p.escapedName === 'actionCreators')
                        const actionCreators = actionCreatorsProperty?.valueDeclaration

                        if (
                            actionCreators &&
                            ts.isPropertySignature(actionCreators) &&
                            ts.isTypeLiteralNode(actionCreators.type)
                        ) {
                            const actionCreator = actionCreators.type.members.find(
                                (m) => (m.name as ts.Identifier)?.escapedText === actionName,
                            )

                            if (
                                actionCreator &&
                                ts.isPropertySignature(actionCreator) &&
                                ts.isFunctionTypeNode(actionCreator.type)
                            ) {
                                const type = checker.getTypeFromTypeNode(actionCreator.type)
                                const signature = type.getCallSignatures()[0]
                                const sigReturnType = signature.getReturnType()
                                const sigReturnTypeNode = checker.typeToTypeNode(
                                    sigReturnType,
                                    undefined,
                                    NodeBuilderFlags.NoTruncation,
                                )
                                gatherImports(sigReturnTypeNode, checker, parsedLogic)

                                if (actionType === '__computed') {
                                    if (ts.isTypeNode(actionCreator.type) && ts.isTypeNode(actionCreator.type.type)) {
                                        const m = (actionCreator.type.type as any).members?.find(
                                            (m) => m.name?.getText() === 'type',
                                        )
                                        if (ts.isPropertySignature(m) && ts.isLiteralTypeNode(m.type)) {
                                            const str = m.type.getText()
                                            if (str) {
                                                actionType = str.substring(1, str.length - 1)
                                            }
                                        }
                                    }
                                }

                                extraActions[actionType] = cloneNodeSorted(actionCreator.type) //payload
                            }
                        }
                    }
                }
            }
        }
    }
    return extraActions
}

export function getLogicPathString(appOptions: AppOptions, fileName: string) {
    let cwd = process.cwd()
    if (appOptions?.rootPath) {
        cwd = path.resolve(cwd, appOptions.rootPath)
    }
    const pathString = path
        .relative(cwd, fileName)
        .replace(/^.\//, '')
        .replace(/\.[jt]sx?$/, '')
        .replace(/\//g, '.')

    return pathString
}

export function getFilenamesForSymbol(symbol: ts.Symbol): string[] | undefined {
    return (symbol?.declarations || []).map((d) => d.getSourceFile().fileName).filter((f) => !rejectImportPath(f))
}

/** gathers onto parsedLogic the TypeReference nodes that are declared in a different sourceFile */
export function gatherImports(input: ts.Node, checker: ts.TypeChecker, parsedLogic: ParsedLogic) {
    if (!input) {
        return
    }
    function getImports(requestedNode) {
        let node = requestedNode
        if (ts.isTypeReferenceNode(node)) {
            let typeRootName: string | undefined
            if (node.typeName?.kind === SyntaxKind.FirstNode) {
                try {
                    typeRootName = node.typeName.getFirstToken().getText()
                } catch (e) {
                    typeRootName = (node.typeName.left as any)?.escapedText
                }
            }
            const symbol = checker.getSymbolAtLocation(node.typeName) || (node.typeName as any).symbol
            if (symbol) {
                storeExtractedSymbol(symbol, checker, parsedLogic, typeRootName)
            }
        }
        ts.forEachChild(requestedNode, getImports)
    }
    getImports(input)
}

export function storeExtractedSymbol(
    symbol: ts.Symbol,
    checker: ts.TypeChecker,
    parsedLogic: ParsedLogic,
    typeRootName?: string,
) {
    const declaration = symbol.getDeclarations()?.[0]
    let typeName = typeRootName

    if (
        declaration &&
        (ts.isImportSpecifier(declaration) ||
            ts.isTypeAliasDeclaration(declaration) ||
            ts.isInterfaceDeclaration(declaration) ||
            ts.isEnumDeclaration(declaration) ||
            ts.isClassDeclaration(declaration))
    ) {
        typeName = typeName || declaration.name.getText()
    }

    // Also checking isTypeNameAlreadyImported to prevent multiple imports of the same type,
    // which we were seeing when importing from `export * from ...` files
    if (typeName && !isTypeNameAlreadyImported(parsedLogic, typeName)) {
        const importFilenameFromNode = getFilenameForNode(declaration, checker)
        const sourceFileImportedPackagePath =
            importFilenameFromNode &&
            findImportPathForPackageInSourceFile(
                parsedLogic.node.getSourceFile(),
                importFilenameFromNode,
                typeName.split('.')[0],
                checker,
            )
        const importFilename = sourceFileImportedPackagePath || importFilenameFromNode
        if (importFilename && !rejectImportPath(importFilename)) {
            addTypeImport(parsedLogic, importFilename, typeName)
        }
    }
}

function findImportPathForPackageInSourceFile(
    sourceFile: ts.SourceFile,
    importFilename: string,
    typeName: string,
    checker: ts.TypeChecker,
): string | undefined {
    const packageName = extractNpmPackageName(importFilename)
    if (!packageName) {
        return
    }

    for (const statement of sourceFile.statements) {
        if (!ts.isImportDeclaration(statement) || !ts.isStringLiteralLike(statement.moduleSpecifier)) {
            continue
        }
        const importPath = statement.moduleSpecifier.text
        if (
            extractNpmPackageName(importPath) === packageName &&
            moduleSpecifierExportsTypeName(statement.moduleSpecifier, typeName, checker)
        ) {
            return importPath
        }
    }
}

function moduleSpecifierExportsTypeName(
    moduleSpecifier: ts.Expression,
    typeName: string,
    checker: ts.TypeChecker,
): boolean {
    const moduleSymbol = checker.getSymbolAtLocation(moduleSpecifier)
    if (!moduleSymbol) {
        return false
    }

    return checker.getExportsOfModule(moduleSymbol).some((moduleExport) => moduleExport.getName() === typeName)
}

function extractNpmPackageName(input: string): string | undefined {
    const packagePath = extractNodeModulesPackagePath(input)
    if (packagePath) {
        const normalizedPackagePath = packagePath.replace(/\\/g, '/')
        if (normalizedPackagePath === 'typescript' || normalizedPackagePath.startsWith('typescript/')) {
            return
        }
        if (normalizedPackagePath.startsWith('@types/')) {
            return
        }
        if (normalizedPackagePath.startsWith('@')) {
            const [scope, name] = normalizedPackagePath.split('/')
            return scope && name ? `${scope}/${name}` : undefined
        }
        return normalizedPackagePath.split('/')[0]
    }

    if (input.startsWith('@types/')) {
        return
    }
    if (input.startsWith('@')) {
        const [scope, name] = input.split('/')
        if (!scope || scope.length <= 1 || !name) {
            return
        }
        return `${scope}/${name}`
    }
    if (!input.startsWith('.') && !input.startsWith('/') && !input.startsWith('~')) {
        return input.split('/')[0]
    }
}

function extractNodeModulesPackagePath(input: string): string | undefined {
    const normalizedInput = input.replace(/\\/g, '/')
    const nodeModulesMarker = '/node_modules/'
    if (!normalizedInput.includes(nodeModulesMarker)) {
        return
    }

    let packagePath = normalizedInput.split(nodeModulesMarker).pop()
    if (!packagePath) {
        return
    }
    if (packagePath.startsWith('.pnpm/')) {
        const pnpmNodeModulesMarker = '/node_modules/'
        if (!packagePath.includes(pnpmNodeModulesMarker)) {
            return
        }
        packagePath = packagePath.split(pnpmNodeModulesMarker).pop()
        if (!packagePath) {
            return
        }
    }
    return packagePath
}

export function getFilenameForImportDeclaration(checker: ts.TypeChecker, importNode: ts.ImportDeclaration): string {
    const moduleSymbol = checker.getSymbolAtLocation(importNode.moduleSpecifier)
    const otherSourceFile = moduleSymbol?.getDeclarations()[0].getSourceFile()
    if (otherSourceFile) {
        return otherSourceFile.fileName || importNode.moduleSpecifier.getText()
    }
}

export function getFilenameForNode(node: ts.Node, checker: ts.TypeChecker): string | void {
    let importNode: ts.Node = node
    while (importNode) {
        if (ts.isImportDeclaration(importNode)) {
            return getFilenameForImportDeclaration(checker, importNode)
        }
        if (isSourceFile(importNode)) {
            return importNode.fileName
        }
        importNode = importNode.parent
    }
}

function addTypeImport(parsedLogic: ParsedLogic, file: string, typeName: string) {
    if (!parsedLogic.typeReferencesToImportFromFiles[file]) {
        parsedLogic.typeReferencesToImportFromFiles[file] = new Set()
    }
    parsedLogic.typeReferencesToImportFromFiles[file].add(typeName.split('.')[0])
}

function isTypeNameAlreadyImported(parsedLogic: ParsedLogic, typeName: string) {
    for (const file in parsedLogic.typeReferencesToImportFromFiles) {
        if (parsedLogic.typeReferencesToImportFromFiles[file].has(typeName)) {
            return true
        }
    }
    return false
}

export function unPromisify(node: ts.Node): ts.Node {
    if (ts.isTypeReferenceNode(node) && (node.typeName as any)?.escapedText === 'Promise') {
        return node.typeArguments?.[0]
    }
    return node
}

export function isAnyUnknown(node?: ts.Node): boolean {
    if (!node) {
        return true
    }
    const unPromised = unPromisify(node)
    return (
        !unPromised ||
        unPromised.kind === SyntaxKind.AnyKeyword ||
        unPromised.kind === SyntaxKind.UnknownKeyword ||
        (ts.isTypeLiteralNode(unPromised) && unPromised.members.length === 0)
    )
}

const syntaxKindOrderOverride = (kind: ts.SyntaxKind): number => {
    if (kind === ts.SyntaxKind.UndefinedKeyword) {
        // sort undefineds at the end always
        return 999
    }
    return kind
}
function compareNodes(a?: ts.Node, b?: ts.Node): number {
    if (!a) {
        if (!b) {
            return 0
        }
        return -1
    } else if (!b) {
        return 1
    }
    if (ts.isLiteralTypeNode(a) && ts.isLiteralTypeNode(b)) {
        if (ts.isStringLiteralLike(a.literal) && ts.isStringLiteralLike(b.literal)) {
            return a.literal.text.localeCompare(b.literal.text)
        } else if (ts.isNumericLiteral(a.literal) && ts.isNumericLiteral(b.literal)) {
            return Number(a.literal.text) - Number(b.literal.text)
        }
        return a.literal.kind - b.literal.kind
    } else if (ts.isTypeReferenceNode(a) && ts.isTypeReferenceNode(b)) {
        try {
            const textA = (a.typeName as any).escapedText || ''
            const textB = (b.typeName as any).escapedText || ''
            return textA.localeCompare(textB)
        } catch (e) {
            return 0
        }
    } else if (ts.isIdentifier(a) && ts.isIdentifier(b)) {
        try {
            const textA = (a as any).escapedText || ''
            const textB = (b as any).escapedText || ''
            return textA.localeCompare(textB)
        } catch (e) {
            return 0
        }
    } else if (ts.isImportTypeNode(a) && ts.isImportTypeNode(b)) {
        const compare = compareNodes(a.argument, b.argument)
        if (compare !== 0) {
            return compare
        }
        return compareNodes(a.qualifier, b.qualifier)
    } else if (ts.isUnionTypeNode(a) && ts.isUnionTypeNode(b)) {
        if (a.types.length === b.types.length) {
            for (let index = 0; index < a.types.length; index++) {
                const compare = compareNodes(a.types[index], b.types[index])
                if (compare !== 0) {
                    return compare
                }
            }
            return 0
        }
        return a.types.length - b.types.length
    } else if (ts.isTypeLiteralNode(a) && ts.isTypeLiteralNode(b)) {
        if (a.members.length === b.members.length) {
            for (let index = 0; index < a.members.length; index++) {
                const compare = compareNodes(a.members[index], b.members[index])
                if (compare !== 0) {
                    return compare
                }
                if (ts.isPropertySignature(a.members[index]) && ts.isPropertySignature(b.members[index])) {
                    const typeCompare = compareNodes(
                        (a.members[index] as ts.PropertySignature).type,
                        (b.members[index] as ts.PropertySignature).type,
                    )
                    if (typeCompare !== 0) {
                        return typeCompare
                    }
                }
            }
            return 0
        }
        return a.members.length - b.members.length
    } else if (ts.isArrayTypeNode(a) && ts.isArrayTypeNode(b)) {
        return compareNodes(a.elementType, b.elementType)
    } else if (ts.isPropertySignature(a) && ts.isPropertySignature(b)) {
        try {
            return a.name.getText().localeCompare(b.name.getText())
        } catch (e) {
            return ((a.name as any).escapedText || '').localeCompare((b.name as any).escapedText || '')
        }
    } else if (b.kind === a.kind) {
        // if (a.kind !== 148 && a.kind !== 152 && a.kind !== 155 && a.kind !== 131) {
        //     debugger
        // }
    }
    return syntaxKindOrderOverride(a.kind) - syntaxKindOrderOverride(b.kind)
}

export function cloneNodeSorted<T extends ts.Node>(node: T): T {
    const visit = (node) => {
        if (ts.isUnionTypeNode(node)) {
            ;(node.types as any as ts.Node[]).sort(compareNodes)
        }
        if (ts.isTypeLiteralNode(node) && node.members.length > 1) {
            ;(node.members as any as ts.Node[]).sort(compareNodes)
        }
        ts.forEachChild(node, visit)
    }
    const cloned = cloneNode(node)
    visit(cloned)
    return cloned
}
