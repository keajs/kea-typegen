import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { gatherImports, getParameterDeclaration } from '../utils'
import { cloneNode } from '@wessberg/ts-clone-node'

export function visitConnect(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const loaderName = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

        if (value && ts.isArrayLiteralExpression(value)) {
            for (let i = 0; i < value.elements.length; i += 2) {
                let logicReference = value.elements[i]

                if (ts.isCallExpression(logicReference)) {
                    logicReference = logicReference.expression
                }

                const connectArray = value.elements[i + 1]

                let lookup: Record<string, string> = {}

                if (connectArray && ts.isArrayLiteralExpression(connectArray)) {
                    const strings = connectArray.elements.map((e: ts.StringLiteral) => e.text)
                    for (const string of strings) {
                        if (string.includes(' as ')) {
                            const parts = string.split(' as ')
                            lookup[parts[0]] = parts[1]
                        } else {
                            lookup[string] = string
                        }
                    }
                }

                const symbol = checker.getSymbolAtLocation(logicReference)
                const otherLogicType = checker.getTypeOfSymbolAtLocation(symbol, logicReference)

                if (loaderName === 'actions') {
                    const actionsForLogic = otherLogicType
                        .getProperties()
                        ?.find((p) => p.getName() === 'actionCreators')

                    if (
                        actionsForLogic &&
                        ts.isPropertySignature(actionsForLogic.valueDeclaration) &&
                        ts.isTypeLiteralNode(actionsForLogic.valueDeclaration.type)
                    ) {
                        const actionTypes = actionsForLogic.valueDeclaration.type.members

                        for (const actionType of actionTypes || []) {
                            if (ts.isPropertySignature(actionType)) {
                                const name = actionType.name.getText()
                                const functionTypeNode = actionType.type

                                if (lookup[name] && ts.isFunctionTypeNode(functionTypeNode)) {
                                    const parameters = functionTypeNode.parameters.map((param) =>
                                        getParameterDeclaration(param),
                                    )
                                    let returnType = functionTypeNode.type

                                    if (ts.isParenthesizedTypeNode(returnType)) {
                                        returnType = returnType.type
                                    }

                                    if (ts.isTypeLiteralNode(returnType)) {
                                        const payload = returnType.members.find((m) => m.name.getText() === 'payload')
                                        if (ts.isPropertySignature(payload)) {
                                            const returnTypeNode = cloneNode(payload.type)
                                            gatherImports(actionType, checker, parsedLogic)

                                            parsedLogic.actions.push({
                                                name: lookup[name],
                                                returnTypeNode,
                                                parameters,
                                            })
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                if (loaderName === 'values' || loaderName === 'props') {
                    const valuesForLogic = otherLogicType.getProperties()?.find((p) => p.getName() === 'values')

                    if (valuesForLogic) {
                        const type = checker.getTypeOfSymbolAtLocation(valuesForLogic, valuesForLogic.valueDeclaration)
                        for (const property of type.getProperties()) {
                            const name = property.getName()
                            if (lookup[name]) {
                                if (ts.isPropertySignature(property.valueDeclaration)) {
                                    const typeNode = property.valueDeclaration.type
                                    gatherImports(typeNode, checker, parsedLogic)
                                    parsedLogic.selectors.push({
                                        name: lookup[name],
                                        typeNode,
                                        functionTypes: [],
                                    })
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
