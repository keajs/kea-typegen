import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { getParameterDeclaration, getTypeNodeForDefaultValue } from '../utils'
import { NodeBuilderFlags } from 'typescript'

function isAnyUnknown(node?: ts.Node): boolean {
    if (!node) {
        return true
    }
    const unPromised = unPromisify(node)
    return !unPromised || unPromised.kind === ts.SyntaxKind.AnyKeyword || unPromised.kind === ts.SyntaxKind.UnknownKeyword || (ts.isTypeLiteralNode(unPromised) && unPromised.members.length === 0)
}

function unPromisify(node: ts.Node): ts.Node {
    if (ts.isTypeReferenceNode(node) && (node.typeName as any)?.escapedText === 'Promise') {
        return node.typeArguments?.[0]
    }
    return node
}

export function visitLoaders(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const loaderName = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

        let defaultValue
        let objectLiteral
        if (ts.isArrayLiteralExpression(value)) {
            defaultValue = value.elements[0]
            objectLiteral = value.elements[1]
        } else if (ts.isObjectLiteralExpression(value)) {
            defaultValue = (value.properties.find(
                (property) => checker.getSymbolAtLocation(property.name)?.getName() === '__default',
            ) as ts.PropertyAssignment)?.initializer
            objectLiteral = value
        }

        const defaultValueTypeNode = getTypeNodeForDefaultValue(defaultValue, checker)

        parsedLogic.reducers.push({ name: loaderName, typeNode: defaultValueTypeNode })
        parsedLogic.reducers.push({
            name: `${loaderName}Loading`,
            typeNode: ts.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword),
        })

        if (objectLiteral) {
            ;(objectLiteral.properties || []).forEach((property: ts.PropertyAssignment) => {
                const loaderActionName = checker.getSymbolAtLocation(property.name)?.getName()
                if (loaderActionName === '__default') {
                    return
                }

                const func = property.initializer
                if (!ts.isFunctionLike(func)) {
                    return
                }

                const param = func.parameters ? (func.parameters[0] as ts.ParameterDeclaration) : null
                const parameters = param ? [getParameterDeclaration(param)] : []

                if (!parsedLogic.actions.find(({ name }) => name === `${loaderActionName}`)) {
                    const returnTypeNode = param?.type || ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
                    parsedLogic.actions.push({ name: `${loaderActionName}`, parameters, returnTypeNode })
                }

                if (!parsedLogic.actions.find(({ name }) => name === `${loaderActionName}Success`)) {
                    let returnTypeNode: ts.TypeNode
                    if (func) {
                        const funcType = checker.getTypeAtLocation(func)
                        const signature = funcType?.getCallSignatures()[0]
                        const sigReturnType = signature?.getReturnType()
                        if (sigReturnType) {
                            const resolvedReturnType = checker.typeToTypeNode(
                                sigReturnType,
                                undefined,
                                NodeBuilderFlags.NoTruncation,
                            )
                            if (!isAnyUnknown(unPromisify(resolvedReturnType))) {
                                returnTypeNode = resolvedReturnType
                            }
                        }
                        if (!returnTypeNode && !isAnyUnknown(func.type)) {
                            returnTypeNode = func.type
                        }
                    }

                    if (!returnTypeNode) {
                        returnTypeNode = defaultValueTypeNode || ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
                    }

                    if (
                        ts.isTypeReferenceNode(returnTypeNode) &&
                        (returnTypeNode.typeName as any)?.escapedText === 'Promise'
                    ) {
                        returnTypeNode = returnTypeNode.typeArguments?.[0]
                    }

                    const successParameters = [
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier(loaderName),
                            undefined,
                            returnTypeNode,
                            undefined,
                        ),
                    ]
                    const successReturnTypeNode = ts.createTypeLiteralNode([
                        ts.createPropertySignature(
                            undefined,
                            ts.createIdentifier(loaderName),
                            undefined,
                            returnTypeNode,
                            undefined,
                        ),
                    ])
                    parsedLogic.actions.push({
                        name: `${loaderActionName}Success`,
                        parameters: successParameters,
                        returnTypeNode: successReturnTypeNode,
                    })
                }

                if (!parsedLogic.actions.find(({ name }) => name === `${loaderActionName}Failure`)) {
                    const failureParameters = [
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier('error'),
                            undefined,
                            ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                            undefined,
                        ),
                    ]
                    const failureReturnTypeNode = ts.createTypeLiteralNode([
                        ts.createPropertySignature(
                            undefined,
                            ts.createIdentifier('error'),
                            undefined,
                            ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                            undefined,
                        ),
                    ])

                    parsedLogic.actions.push({
                        name: `${loaderActionName}Failure`,
                        parameters: failureParameters,
                        returnTypeNode: failureReturnTypeNode,
                    })
                }
            })
        }
    }
}
