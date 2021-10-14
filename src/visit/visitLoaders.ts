import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import {
    gatherImports,
    getParameterDeclaration,
    getAndGatherTypeNodeForDefaultValue,
    isAnyUnknown,
    unPromisify,
} from '../utils'
import { NodeBuilderFlags } from 'typescript'

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

        const defaultValueTypeNode = getAndGatherTypeNodeForDefaultValue(defaultValue, checker, parsedLogic)

        gatherImports(defaultValueTypeNode, checker, parsedLogic)

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

                const param = func.parameters ? func.parameters[0] : null
                const parameters = param ? [getParameterDeclaration(param)] : []

                if (!parsedLogic.actions.find(({ name }) => name === `${loaderActionName}`)) {
                    const returnTypeNode = param?.type || ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
                    gatherImports(param, checker, parsedLogic)

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

                    gatherImports(returnTypeNode, checker, parsedLogic)

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
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier('payload'),
                            undefined,
                            parsedLogic.actions.find((a) => a.name === loaderActionName)?.returnTypeNode,
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
                        ts.createPropertySignature(
                            undefined,
                            ts.createIdentifier('payload'),
                            undefined,
                            parsedLogic.actions.find((a) => a.name === loaderActionName)?.returnTypeNode,
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
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier('errorObject'),
                            ts.createToken(ts.SyntaxKind.QuestionToken),
                            ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
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
                        ts.createPropertySignature(
                            undefined,
                            ts.createIdentifier('errorObject'),
                            ts.createToken(ts.SyntaxKind.QuestionToken),
                            ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
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
