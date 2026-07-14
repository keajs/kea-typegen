import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { NodeBuilderFlags } from 'typescript'
import { cloneNodeSorted, gatherImports, getParameterDeclaration } from '../utils'
import { Expression, Type } from 'typescript'

export function visitConnect(parsedLogic: ParsedLogic, type: Type, expression: Expression) {
    const { checker } = parsedLogic

    // passing just one argument, connect(otherLogic)
    const objectTypeAlias = type.aliasSymbol?.name
    if (objectTypeAlias === 'LogicWrapper' || objectTypeAlias === 'BuiltLogic') {
        return
    }

    for (const property of type.getProperties()) {
        const loaderName = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

        if (value && ts.isArrayLiteralExpression(value)) {
            for (let i = 0; i < value.elements.length; i += 2) {
                let logicReference = value.elements[i]

                if (ts.isCallExpression(logicReference)) {
                    logicReference = logicReference.expression
                }

                if (!logicReference) {
                    // nothing to do
                    continue
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
                const sourceLogic = ts.isIdentifier(logicReference) ? logicReference.text : logicReference.getText()

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
                                            const returnTypeNode = cloneNodeSorted(payload.type)
                                            gatherImports(actionType, checker, parsedLogic)

                                            parsedLogic.actions.push({
                                                name: lookup[name],
                                                returnTypeNode,
                                                parameters,
                                                sourceLogic,
                                            })
                                        }
                                    }
                                }
                            }
                        }
                    } else if (actionsForLogic) {
                        // `actionCreators` is not a literal type node we can walk syntactically - e.g. the other
                        // logic is typed with MakeLogicType, where actionCreators is a mapped type. Resolve the
                        // connected action types semantically instead.
                        const actionCreatorsType = checker.getTypeOfSymbolAtLocation(actionsForLogic, logicReference)
                        for (const actionProperty of actionCreatorsType.getProperties()) {
                            const name = actionProperty.getName()
                            if (!lookup[name]) {
                                continue
                            }
                            const actionCreatorType = checker.getTypeOfSymbolAtLocation(actionProperty, logicReference)
                            const signature = actionCreatorType.getCallSignatures()[0]
                            if (!signature) {
                                continue
                            }
                            const signatureNode = checker.signatureToSignatureDeclaration(
                                signature,
                                ts.SyntaxKind.FunctionType,
                                logicReference,
                                NodeBuilderFlags.NoTruncation | NodeBuilderFlags.IgnoreErrors,
                            ) as ts.FunctionTypeNode | undefined
                            if (!signatureNode) {
                                continue
                            }
                            const parameters = signatureNode.parameters.map((param) => getParameterDeclaration(param))
                            let returnType: ts.TypeNode = signatureNode.type
                            if (ts.isParenthesizedTypeNode(returnType)) {
                                returnType = returnType.type
                            }
                            if (ts.isTypeLiteralNode(returnType)) {
                                const payload = returnType.members.find(
                                    (m) => m.name && ts.isIdentifier(m.name) && m.name.text === 'payload',
                                )
                                if (payload && ts.isPropertySignature(payload) && payload.type) {
                                    const returnTypeNode = cloneNodeSorted(payload.type)
                                    gatherImports(signatureNode, checker, parsedLogic)

                                    parsedLogic.actions.push({
                                        name: lookup[name],
                                        returnTypeNode,
                                        parameters,
                                        sourceLogic,
                                    })
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
                                        sourceLogic,
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
