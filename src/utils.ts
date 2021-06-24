import * as ts from 'typescript'
import * as path from 'path'
import { cloneNode } from '@wessberg/ts-clone-node'
import { visitProgram } from './visit/visit'
import { parsedLogicToTypeString } from './print/print'
import { AppOptions, NameType, ParsedLogic } from './types'
import { NodeBuilderFlags } from 'typescript'

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

export function getAndGatherTypeNodeForDefaultValue(
    defaultValue: ts.Node,
    checker: ts.TypeChecker,
    parsedLogic: ParsedLogic,
): ts.TypeNode {
    let typeNode
    if (defaultValue) {
        if (ts.isAsExpression(defaultValue)) {
            gatherImports(defaultValue.type, checker, parsedLogic)
            typeNode = cloneNode(defaultValue.type)
            if (ts.isParenthesizedTypeNode(typeNode)) {
                typeNode = typeNode.type
            }
        } else if (
            defaultValue?.kind === ts.SyntaxKind.TrueKeyword ||
            defaultValue?.kind === ts.SyntaxKind.FalseKeyword
        ) {
            typeNode = ts.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword)
        } else if (ts.isStringLiteralLike(defaultValue)) {
            typeNode = ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword)
        } else if (ts.isNumericLiteral(defaultValue)) {
            typeNode = ts.createKeywordTypeNode(ts.SyntaxKind.NumberKeyword)
        } else if (ts.isArrayLiteralExpression(defaultValue) && defaultValue.elements.length === 0) {
            typeNode = ts.createArrayTypeNode(ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword))
        } else {
            const foundTypeNode = checker.typeToTypeNode(checker.getTypeAtLocation(defaultValue), undefined, undefined)
            gatherImports(foundTypeNode, checker, parsedLogic)
            typeNode = cloneNode(foundTypeNode)
        }
    } else {
        typeNode = ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
    }
    return typeNode
}

export function getParameterDeclaration(param: ts.ParameterDeclaration) {
    return ts.createParameter(
        undefined,
        undefined,
        undefined,
        ts.createIdentifier(param.name.getText()),
        param.initializer || param.questionToken ? ts.createToken(ts.SyntaxKind.QuestionToken) : undefined,
        param.type || ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
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
                    const actionType = nameSymbol.escapedName as string

                    if (ts.isPropertyAccessExpression(expression)) {
                        // expression.expression ==> githubLogic.actionTypes
                        // expression.name ==> setRepositories

                        const symbol = checker.getSymbolAtLocation(expression.expression)
                        const symbolType = checker.getTypeOfSymbolAtLocation(symbol, expression.expression)

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
    return symbol?.declarations
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
            const symbol = checker.getSymbolAtLocation(node.typeName) || (node.typeName as any).symbol
            if (symbol) {
                storeExtractedSymbol(symbol, checker, parsedLogic)
            }
        }
        ts.forEachChild(requestedNode, getImports)
    }
    getImports(input)
}

export function storeExtractedSymbol(symbol: ts.Symbol, checker: ts.TypeChecker, parsedLogic: ParsedLogic) {
    const declaration = symbol.getDeclarations()[0]

    if (ts.isImportSpecifier(declaration)) {
        const importFilename = getFilenameForImportSpecifier(declaration, checker)
        if (importFilename) {
            addTypeImport(parsedLogic, importFilename, declaration.getText())
        } else {
            parsedLogic.typeReferencesInLogicInput.add(declaration.getText())
        }
        return
    }

    const files = getFilenamesForSymbol(symbol)
    if (files.length === 1) {
        // same file, add to logicType<...>
        if (
            ts.isTypeAliasDeclaration(declaration) ||
            ts.isInterfaceDeclaration(declaration) ||
            ts.isEnumDeclaration(declaration)  ||
            ts.isClassDeclaration(declaration)
        ) {
            if (files[0] === parsedLogic.fileName) {
                parsedLogic.typeReferencesInLogicInput.add(declaration.name.getText())
            } else {
                // but is it exported?
                addTypeImport(parsedLogic, files[0], declaration.name.getText())
            }
        }
    }
}

export function getFilenameForImportSpecifier(
    declaration: ts.ImportSpecifier,
    checker: ts.TypeChecker,
): string | void {
    let importNode: ts.Node = declaration
    while (importNode && !ts.isImportDeclaration(importNode)) {
        importNode = importNode.parent
    }
    if (ts.isImportDeclaration(importNode)) {
        const moduleSymbol = checker.getSymbolAtLocation(importNode.moduleSpecifier)
        const otherSourceFile = moduleSymbol?.getDeclarations()[0].getSourceFile()
        if (otherSourceFile) {
            return otherSourceFile.fileName || importNode.moduleSpecifier.getText()
        }
    }
}

function addTypeImport(parsedLogic: ParsedLogic, file: string, typeName: string) {
    if (!parsedLogic.typeReferencesToImportFromFiles[file]) {
        parsedLogic.typeReferencesToImportFromFiles[file] = new Set()
    }
    parsedLogic.typeReferencesToImportFromFiles[file].add(typeName)
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
        unPromised.kind === ts.SyntaxKind.AnyKeyword ||
        unPromised.kind === ts.SyntaxKind.UnknownKeyword ||
        (ts.isTypeLiteralNode(unPromised) && unPromised.members.length === 0)
    )
}
