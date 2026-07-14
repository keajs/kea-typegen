import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { Expression, NodeBuilderFlags, Type } from 'typescript'
import { cloneNodeSorted, gatherImports } from '../utils'

export function visitSelectors(parsedLogic: ParsedLogic, type: Type, expression: Expression) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer
        if (ts.isArrayLiteralExpression(value) && value.elements.length > 1) {
            const inputFunction = value.elements[0] as ts.ArrowFunction | ts.FunctionDeclaration
            const inputFunctionTypeNode = checker.getTypeAtLocation(inputFunction)

            const selectorInputFunctionType = inputFunctionTypeNode.getCallSignatures()[0]?.getReturnType() as ts.Type
            // IgnoreErrors: the tuple may mention types not referenceable from the logic file
            // (e.g. kea's internal Props); we only consume the elements' return types anyway
            const selectorInputTypeNode = selectorInputFunctionType
                ? checker.typeToTypeNode(
                      selectorInputFunctionType,
                      inputFunction,
                      NodeBuilderFlags.NoTruncation | NodeBuilderFlags.IgnoreErrors,
                  )
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
                                ? cloneNodeSorted(selectorTypeNode.type)
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
                    typeNode = cloneNodeSorted(computedFunction.type)
                } else if (type) {
                    typeNode = checker.typeToTypeNode(type, undefined, NodeBuilderFlags.NoTruncation)
                    gatherImports(typeNode, checker, parsedLogic)
                    typeNode = cloneNodeSorted(typeNode)
                } else {
                    typeNode = ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
                }

                parsedLogic.selectors.push({
                    name,
                    typeNode,
                    functionTypes,
                })

                // combiner parameters without a type annotation, e.g. `(counter) => counter * 2`.
                // In inline mode these get their inferred types written into the source, since
                // MakeLogicType's loose selector typing can no longer infer them contextually.
                computedFunction.parameters.forEach((parameter, index) => {
                    const functionType = functionTypes[index]
                    if (
                        !parameter.type &&
                        !parameter.dotDotDotToken &&
                        ts.isIdentifier(parameter.name) &&
                        functionType &&
                        functionType.type.kind !== ts.SyntaxKind.AnyKeyword
                    ) {
                        parsedLogic.selectorParamAnnotations.push({
                            parameter,
                            typeNode: functionType.type,
                        })
                    }
                })
            }
        }
    }
}
