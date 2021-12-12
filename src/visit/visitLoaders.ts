import { ParsedLogic } from '../types'
import {
    factory,
    isArrayLiteralExpression,
    isFunctionLike,
    isObjectLiteralExpression,
    isTypeReferenceNode,
    PropertyAssignment,
    SyntaxKind,
    Type,
    TypeNode,
} from 'typescript'
import {
    gatherImports,
    getParameterDeclaration,
    getAndGatherTypeNodeForDefaultValue,
    isAnyUnknown,
    unPromisify,
} from '../utils'
import { NodeBuilderFlags } from 'typescript'

export function visitLoaders(type: Type, inputProperty: PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const loaderName = property.getName()
        const value = (property.valueDeclaration as PropertyAssignment).initializer

        let defaultValue
        let objectLiteral
        if (isArrayLiteralExpression(value)) {
            defaultValue = value.elements[0]
            objectLiteral = value.elements[1]
        } else if (isObjectLiteralExpression(value)) {
            defaultValue = (value.properties.find(
                (property) => checker.getSymbolAtLocation(property.name)?.getName() === '__default',
            ) as PropertyAssignment)?.initializer
            objectLiteral = value
        }

        const defaultValueTypeNode = getAndGatherTypeNodeForDefaultValue(defaultValue, checker, parsedLogic)

        gatherImports(defaultValueTypeNode, checker, parsedLogic)

        parsedLogic.reducers.push({ name: loaderName, typeNode: defaultValueTypeNode })
        parsedLogic.reducers.push({
            name: `${loaderName}Loading`,
            typeNode: factory.createKeywordTypeNode(SyntaxKind.BooleanKeyword),
        })

        if (objectLiteral) {
            ;(objectLiteral.properties || []).forEach((property: PropertyAssignment) => {
                const loaderActionName = checker.getSymbolAtLocation(property.name)?.getName()
                if (loaderActionName === '__default') {
                    return
                }

                const func = property.initializer
                if (!isFunctionLike(func)) {
                    return
                }

                const param = func.parameters ? func.parameters[0] : null
                const parameters = param ? [getParameterDeclaration(param)] : []

                if (!parsedLogic.actions.find(({ name }) => name === `${loaderActionName}`)) {
                    const returnTypeNode = param?.type || factory.createKeywordTypeNode(SyntaxKind.AnyKeyword)
                    gatherImports(param, checker, parsedLogic)

                    parsedLogic.actions.push({ name: `${loaderActionName}`, parameters, returnTypeNode })
                }

                if (!parsedLogic.actions.find(({ name }) => name === `${loaderActionName}Success`)) {
                    let returnTypeNode: TypeNode
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
                        returnTypeNode = defaultValueTypeNode || factory.createKeywordTypeNode(SyntaxKind.AnyKeyword)
                    }

                    if (
                        isTypeReferenceNode(returnTypeNode) &&
                        (returnTypeNode.typeName as any)?.escapedText === 'Promise'
                    ) {
                        returnTypeNode = returnTypeNode.typeArguments?.[0]
                    }

                    gatherImports(returnTypeNode, checker, parsedLogic)

                    const successParameters = [
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier(loaderName),
                            undefined,
                            returnTypeNode,
                            undefined,
                        ),
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('payload'),
                            factory.createToken(SyntaxKind.QuestionToken),
                            parsedLogic.actions.find((a) => a.name === loaderActionName)?.returnTypeNode,
                            undefined,
                        ),
                    ]
                    const successReturnTypeNode = factory.createTypeLiteralNode([
                        factory.createPropertySignature(
                            undefined,
                            factory.createIdentifier(loaderName),
                            undefined,
                            returnTypeNode,
                        ),
                        factory.createPropertySignature(
                            undefined,
                            factory.createIdentifier('payload'),
                            factory.createToken(SyntaxKind.QuestionToken),
                            parsedLogic.actions.find((a) => a.name === loaderActionName)?.returnTypeNode,
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
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('error'),
                            undefined,
                            factory.createKeywordTypeNode(SyntaxKind.StringKeyword),
                            undefined,
                        ),
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('errorObject'),
                            factory.createToken(SyntaxKind.QuestionToken),
                            factory.createKeywordTypeNode(SyntaxKind.AnyKeyword),
                            undefined,
                        ),
                    ]
                    const failureReturnTypeNode = factory.createTypeLiteralNode([
                        factory.createPropertySignature(
                            undefined,
                            factory.createIdentifier('error'),
                            undefined,
                            factory.createKeywordTypeNode(SyntaxKind.StringKeyword),
                        ),
                        factory.createPropertySignature(
                            undefined,
                            factory.createIdentifier('errorObject'),
                            factory.createToken(SyntaxKind.QuestionToken),
                            factory.createKeywordTypeNode(SyntaxKind.AnyKeyword),
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
