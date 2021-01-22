import * as ts from 'typescript'
import * as path from 'path'
import { cloneNode } from '@wessberg/ts-clone-node'
import { visitProgram } from './visit/visit'
import { parsedLogicToTypeString } from './print/print'
import { AppOptions, NameType, ParsedLogic } from './types'

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

    if (!node.parent || !ts.isCallExpression(node.parent)) {
        return false
    }

    const symbol = checker.getSymbolAtLocation(node)
    if (!symbol || symbol.getName() !== 'kea') {
        return false
    }

    const input = node.parent.arguments[0]

    if (!ts.isObjectLiteralExpression(input)) {
        return false
    }

    return true
}

export function getTypeNodeForDefaultValue(defaultValue: ts.Node, checker: ts.TypeChecker): ts.TypeNode {
    let typeNode
    if (defaultValue) {
        if (ts.isAsExpression(defaultValue)) {
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
            typeNode = cloneNode(checker.typeToTypeNode(checker.getTypeAtLocation(defaultValue), undefined, undefined))
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
        if (!newReducers[reducer.name] || ts.SyntaxKind[reducer.typeNode.kind] === '‌AnyKeyword') {
            newReducers[reducer.name] = reducer
        }
    }

    return Object.values(newReducers)
}

export function extractImportedActions(
    actionObjects: ts.Expression | ts.ObjectLiteralExpression,
    checker: ts.TypeChecker,
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
