import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { getParameterDeclaration, getTypeNodeForDefaultValue } from '../utils'

export function visitLoaders(type: ts.Type, parsedLogic: ParsedLogic) {
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

        const typeNode = getTypeNodeForDefaultValue(defaultValue, checker)

        parsedLogic.reducers.push({ name: loaderName, typeNode })
        parsedLogic.reducers.push({
            name: `${loaderName}Loading`,
            typeNode: ts.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword),
        })

        if (objectLiteral) {
            objectLiteral.properties.forEach((property: ts.PropertyAssignment) => {
                const loaderName = checker.getSymbolAtLocation(property.name)?.getName()
                if (loaderName === '__default') {
                    return
                }
                const func = property.initializer as ts.ArrowFunction
                const param = func.parameters[0] as ts.ParameterDeclaration
                if (!parsedLogic.actions.find(({ name }) => name === `${loaderName}`)) {
                    const parameters = param ? [getParameterDeclaration(param)] : []
                    const returnTypeNode = param?.type || ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
                    parsedLogic.actions.push({name: `${loaderName}`, parameters, returnTypeNode})
                }
                if (!parsedLogic.actions.find(({ name }) => name === `${loaderName}Success`)) {
                    const successParameters = [
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier(loaderName),
                            undefined,
                            typeNode,
                            undefined,
                        ),
                    ]
                    const successReturnTypeNode = ts.createTypeLiteralNode([
                        ts.createPropertySignature(
                            undefined,
                            ts.createIdentifier(loaderName),
                            undefined,
                            typeNode,
                            undefined,
                        ),
                    ])
                    parsedLogic.actions.push({
                        name: `${loaderName}Success`,
                        parameters: successParameters,
                        returnTypeNode: successReturnTypeNode,
                    })
                }

                if (!parsedLogic.actions.find(({ name }) => name === `${loaderName}Failure`)) {
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
                        name: `${loaderName}Failure`,
                        parameters: failureParameters,
                        returnTypeNode: failureReturnTypeNode,
                    })
                }
            })
        }
    }
}
