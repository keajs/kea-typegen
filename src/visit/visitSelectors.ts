import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { NodeBuilderFlags } from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'
import { gatherImports } from '../utils'

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
                functionTypes = (selectorInputTypeNode.elements || ts.factory.createNodeArray([]))
                    .filter((e) => ts.isTypeNode(e))
                    .map((selectorTypeNode, index) => {
                        let name = functionNames[index] || 'arg'
                        takenNames[name] = (takenNames[name] || 0) + 1
                        if (takenNames[name] > 1) {
                            name = `${name}${takenNames[name]}`
                        }
                        if (ts.isFunctionTypeNode(selectorTypeNode)) {
                            gatherImports(selectorTypeNode.type, checker, parsedLogic)
                        }
                        return {
                            name,
                            type: ts.isFunctionTypeNode(selectorTypeNode)
                                ? cloneNode(selectorTypeNode.type)
                                : ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        }
                    })
            }

            // return type
            const computedFunction = value.elements[1]
            if (ts.isFunctionLike(computedFunction)) {
                const type = checker.getReturnTypeOfSignature(checker.getSignatureFromDeclaration(computedFunction))

                let typeNode: ts.TypeNode
                if (computedFunction.type) {
                    gatherImports(computedFunction.type, checker, parsedLogic)
                    typeNode = cloneNode(computedFunction.type)
                } else if (type) {
                    typeNode = checker.typeToTypeNode(type, undefined, NodeBuilderFlags.NoTruncation)
                    gatherImports(typeNode, checker, parsedLogic)
                } else {
                    typeNode = ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
                }

                parsedLogic.selectors.push({
                    name,
                    typeNode,
                    functionTypes,
                })
            }
        }
    }
}
