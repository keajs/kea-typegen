import * as ts from 'typescript'
import { visitProgram } from './visit/visit'
import { parsedLogicToTypeString } from './print/print'

export function logicSourceToLogicType(logicSource: string) {
    const program = programFromSource(logicSource)
    const [parsedLogic] = visitProgram(program)
    return parsedLogicToTypeString(parsedLogic)
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

    if (!ts.isCallExpression(node.parent)) {
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
