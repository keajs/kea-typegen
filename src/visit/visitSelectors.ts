import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'
import { NodeBuilderFlags } from 'typescript'

export function visitSelectors(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer
        if (ts.isArrayLiteralExpression(value) && value.elements.length > 1) {
            const inputFunction = value.elements[0] as ts.ArrowFunction | ts.FunctionDeclaration
            const inputFunctionTypeNode = checker.getTypeAtLocation(inputFunction)

            const selectorInputFunctionType = inputFunctionTypeNode.getCallSignatures()[0]?.getReturnType() as ts.Type
            const selectorInputTypeNode = selectorInputFunctionType
                ? checker.typeToTypeNode(selectorInputFunctionType, inputFunction, NodeBuilderFlags.NoTruncation)
                : null

            let functionNames = []
            if (ts.isArrayLiteralExpression(inputFunction.body)) {
                functionNames = inputFunction.body.elements.map((element) => {
                    if (ts.isPropertyAccessExpression(element)) {
                        return element.name.getText()
                    } else {
                        return null
                    }
                })
            }

            let functionTypes = []

            if (selectorInputTypeNode && ts.isTupleTypeNode(selectorInputTypeNode)) {
                let takenNames: Record<string, number> = {}
                functionTypes = selectorInputTypeNode.elementTypes.map((selectorTypeNode, index) => {
                    let name = functionNames[index] || 'arg'
                    takenNames[name] = (takenNames[name] || 0) + 1
                    if (takenNames[name] > 1) {
                        name = `${name}${takenNames[name]}`
                    }

                    return {
                        name,
                        type: ts.isFunctionTypeNode(selectorTypeNode)
                            ? cloneNode(selectorTypeNode.type)
                            : ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                    }
                })
            }

            // return type
            const computedFunction = value.elements[1] as ts.ArrowFunction | ts.FunctionDeclaration
            const computedFunctionTypeNode = checker.getTypeAtLocation(computedFunction)
            const type = computedFunctionTypeNode.getCallSignatures()[0]?.getReturnType()

            parsedLogic.selectors.push({
                name,
                typeNode: type
                    ? checker.typeToTypeNode(type, undefined, undefined)
                    : ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                functionTypes,
            })
        }
    }
}
