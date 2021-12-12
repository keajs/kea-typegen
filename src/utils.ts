import * as ts from 'typescript'
import * as path from 'path'
import { cloneNode } from '@wessberg/ts-clone-node'
import { visitProgram } from './visit/visit'
import { parsedLogicToTypeString } from './print/print'
import { AppOptions, NameType, ParsedLogic } from './types'
import { factory, NodeBuilderFlags, SyntaxKind } from 'typescript'

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

    if (!input || !ts.isObjectLiteralExpression(input)) {
        return false
    }

    return true
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
            typeNode = checker.typeToTypeNode(checker.getTypeAtLocation(node), undefined, undefined)
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
    return cloneNode(typeNode)
}

export function getParameterDeclaration(param: ts.ParameterDeclaration) {
    return factory.createParameterDeclaration(
        undefined,
        undefined,
        undefined,
        factory.createIdentifier(param.name.getText()),
        param.initializer || param.questionToken ? factory.createToken(SyntaxKind.QuestionToken) : undefined,
        cloneNode(param.type || factory.createKeywordTypeNode(SyntaxKind.AnyKeyword)),
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

                                extraActions[actionType] = cloneNode(actionCreator.type) //payload
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
    return (symbol?.declarations || [])
        .map((d) => d.getSourceFile().fileName)
        .filter((str) => !str.includes('/node_modules/typescript/lib/lib'))
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

    if (declaration && ts.isImportSpecifier(declaration)) {
        const importFilename = getFilenameForImportSpecifier(declaration, checker)
        if (importFilename) {
            addTypeImport(parsedLogic, importFilename, typeRootName || declaration.getText())
        } else {
            parsedLogic.typeReferencesInLogicInput.add(typeRootName || declaration.getText())
        }
        return
    }

    const files = getFilenamesForSymbol(symbol)
    if (files[0]) {
        // same file, add to logicType<...>
        if (
            ts.isTypeAliasDeclaration(declaration) ||
            ts.isInterfaceDeclaration(declaration) ||
            ts.isEnumDeclaration(declaration) ||
            ts.isClassDeclaration(declaration)
        ) {
            if (files[0] === parsedLogic.fileName) {
                parsedLogic.typeReferencesInLogicInput.add(typeRootName || declaration.name.getText())
            } else {
                // but is it exported?
                addTypeImport(parsedLogic, files[0], typeRootName || declaration.name.getText())
            }
        }
    }
}

export function getFilenameForImportDeclaration(checker: ts.TypeChecker, importNode: ts.ImportDeclaration): string {
    const moduleSymbol = checker.getSymbolAtLocation(importNode.moduleSpecifier)
    const otherSourceFile = moduleSymbol?.getDeclarations()[0].getSourceFile()
    if (otherSourceFile) {
        return otherSourceFile.fileName || importNode.moduleSpecifier.getText()
    }
}

export function getFilenameForImportSpecifier(declaration: ts.ImportSpecifier, checker: ts.TypeChecker): string | void {
    let importNode: ts.Node = declaration
    while (importNode && !ts.isImportDeclaration(importNode)) {
        importNode = importNode.parent
    }
    if (ts.isImportDeclaration(importNode)) {
        return getFilenameForImportDeclaration(checker, importNode)
    }
}

function addTypeImport(parsedLogic: ParsedLogic, file: string, typeName: string) {
    if (!parsedLogic.typeReferencesToImportFromFiles[file]) {
        parsedLogic.typeReferencesToImportFromFiles[file] = new Set()
    }
    parsedLogic.typeReferencesToImportFromFiles[file].add(typeName.split('.')[0])
}

export function arrayContainsSet(array: string[], setToContain: Set<string>): boolean {
    const arraySet = new Set(array)
    for (const str of setToContain) {
        if (!arraySet.has(str)) {
            return false
        }
    }
    return true
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
