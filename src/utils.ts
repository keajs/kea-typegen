import * as ts from 'typescript'
import * as path from 'path'
import { visitProgram } from './visit/visit'
import { parsedLogicToTypeString } from './print/print'
import {AppOptions, NameType, ParsedLogic, ReducerTransform} from './types'

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
            typeNode = defaultValue.type
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
        } else {
            typeNode = checker.typeToTypeNode(checker.getTypeAtLocation(defaultValue))
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

export function getPathName(cwd: string, parsedLogic: ParsedLogic) {
    const pathName = path
        .relative(cwd, parsedLogic.fileName)
        .replace(/^.\//, '')
        .replace(/\.[jt]sx?$/, '')
        .replace(/\//g, '.')
    return pathName
}

export const toSpaces = (key) => key.replace(/(?:^|\.?)([A-Z])/g, (x, y) => ' ' + y.toLowerCase()).replace(/^ /, '')

export function getActionTypeCreator(appOptions: AppOptions, parsedLogic: ParsedLogic) {
    return function (actionName) {
        let cwd = process.cwd()

        if (appOptions?.logicStartPath) {
            cwd = path.resolve(cwd, appOptions.logicStartPath)
        }
        const pathName = getPathName(cwd, parsedLogic)

        return `${toSpaces(actionName)} (${pathName})`
    }
}

export function combineExtraActions(reducers: ReducerTransform[]) {
    const extraActions: Record<string, ts.TypeNode> = {}

    if (reducers) {
        reducers.forEach((reducer) => {
            if (reducer.extraActions) {
                Object.entries(reducer.extraActions).forEach(([type, payload]) => extraActions[type] = payload)
            }
        })
    }

    return extraActions
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

