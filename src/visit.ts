import * as ts from 'typescript'
import { ParsedLogic } from './types'
import { isKeaCall } from './utils'

export function visitProgram(program: ts.Program) {
    const checker = program.getTypeChecker()
    const parsedLogics: ParsedLogic[] = []

    for (const sourceFile of program.getSourceFiles()) {
        if (!sourceFile.isDeclarationFile) {
            ts.forEachChild(sourceFile, createVisit(checker, parsedLogics, sourceFile))
        }
    }

    return parsedLogics
}

export function createVisit(checker: ts.TypeChecker, parsedLogics: ParsedLogic[], sourceFile: ts.SourceFile) {
    return function visit(node: ts.Node) {
        if (!isKeaCall(node, checker)) {
            ts.forEachChild(node, visit)
            return
        }

        let logicName = 'logic'
        if (ts.isCallExpression(node.parent) && ts.isVariableDeclaration(node.parent.parent)) {
            logicName = node.parent.parent.name.getText()
        }

        const parsedLogic: ParsedLogic = {
            checker,
            logicName,
            fileName: sourceFile.fileName,
            actions: [],
            reducers: [],
        }

        const input = (node.parent as ts.CallExpression).arguments[0] as ts.ObjectLiteralExpression

        for (const inputProperty of input.properties) {
            const symbol = checker.getSymbolAtLocation(inputProperty.name as ts.Identifier)

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
                    const name = property.getName()
                    const type = checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration!)
                    const typeNode = checker.typeToTypeNode(type) as ts.TypeNode
                    const signature = type.getCallSignatures()[0]

                    parsedLogic.actions.push({ name, type, typeNode, signature })
                }
            } else if (name === 'reducers') {
                for (const property of type.getProperties()) {
                    const name = property.getName()
                    const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

                    if (ts.isArrayLiteralExpression(value)) {
                        const defaultValue = value.elements[0]

                        if (ts.isAsExpression(defaultValue)) {
                            let typeNode = defaultValue.type
                            if (ts.isParenthesizedTypeNode(typeNode)) {
                                typeNode = typeNode.type
                            }
                            parsedLogic.reducers.push({ name, typeNode })
                        } else if (ts.isStringLiteralLike(defaultValue)) {
                            parsedLogic.reducers.push({
                                name,
                                typeNode: ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                            })
                        } else if (ts.isNumericLiteral(defaultValue)) {
                            parsedLogic.reducers.push({
                                name,
                                typeNode: ts.createKeywordTypeNode(ts.SyntaxKind.NumberKeyword),
                            })
                        } else {
                            const type = checker.getTypeAtLocation(defaultValue)
                            parsedLogic.reducers.push({ name, type, typeNode: checker.typeToTypeNode(type) })
                        }
                    } else if (ts.isObjectLiteralExpression(value)) {
                        parsedLogic.reducers.push({
                            name,
                            typeNode: ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        })
                    }
                }
            }
        }

        parsedLogics.push(parsedLogic)
    }
}
