import * as ts from 'typescript'
import {visitProgram} from "./visit/visit";
import {logicToTypeString} from "./print/print";

export function logicSourceToLogicType(logicSource: string) {
    const program = programFromSource(logicSource)
    const [parsedLogic] = visitProgram(program)
    return logicToTypeString(parsedLogic)
}

export function sourceToSourceFile(sourceCode: string, filename: string = 'logic.ts') {
    return ts.createSourceFile(filename, sourceCode, ts.ScriptTarget.ES5, true)
}

export function programFromSource(sourceCode: string) {
    const options = {}
    const compilerHost = ts.createCompilerHost(options)
    compilerHost.getSourceFile = fileName => fileName === 'logic.ts' ? sourceToSourceFile(sourceCode) : undefined
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
