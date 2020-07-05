import * as ts from 'typescript'
import { parseActionProperty, parseReducerProperty } from './parse'
import { ParsedLogic } from './types'
import { isKeaCall } from './utils'

export function visitProgram(program: ts.Program) {
    const checker = program.getTypeChecker()
    const parsedLogics: ParsedLogic[] = []
    const visit = createVisit(checker, parsedLogics)

    for (const sourceFile of program.getSourceFiles()) {
        if (!sourceFile.isDeclarationFile) {
            ts.forEachChild(sourceFile, visit)
        }
    }

    return parsedLogics
}

export function createVisit(checker: ts.TypeChecker, parsedLogics: ParsedLogic[]) {
    return function visit(node: ts.Node) {
        if (!isKeaCall(node, checker)) {
            ts.forEachChild(node, visit)
            return
        }

        const parsedLogic: ParsedLogic = {
            actions: [],
            reducers: [],
            checker: checker,
        }

        const input = (node.parent as ts.CallExpression).arguments[0] as ts.ObjectLiteralExpression

        for (const property of input.properties) {
            const symbol = checker.getSymbolAtLocation(property.name as ts.Identifier)

            if (!symbol) {
                continue
            }

            const name = symbol.getName()
            let type = checker.getTypeOfSymbolAtLocation(symbol, symbol.valueDeclaration!)

            if (ts.isFunctionTypeNode(checker.typeToTypeNode(type) as ts.TypeNode)) {
                type = type.getCallSignatures()[0].getReturnType()
            }

            if (name === 'actions') {
                const properties = checker.getPropertiesOfType(type)

                for (const property of properties) {
                    parseActionProperty(property, checker, parsedLogic)
                }
            } else if (name === 'reducers') {
                for (const property of type.getProperties()) {
                    parseReducerProperty(property, checker, parsedLogic)
                }
            }
        }

        parsedLogics.push(parsedLogic)
    }
}
