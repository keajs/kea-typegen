import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function visitReducers(type: ts.Type, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

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
