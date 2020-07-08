import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function visitSelectors(type: ts.Type, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer
        if (ts.isArrayLiteralExpression(value)) {
            const computedFunction = value.elements[1] as ts.ArrowFunction | ts.FunctionDeclaration
            const computedFunctionTypeNode = checker.getTypeAtLocation(computedFunction)
            const type = computedFunctionTypeNode.getCallSignatures()[0].getReturnType()

            parsedLogic.selectors.push({
                name,
                typeNode: checker.typeToTypeNode(type)
            })
        }
    }
}
