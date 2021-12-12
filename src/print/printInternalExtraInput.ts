import { ParsedLogic } from '../types'
import { factory } from 'typescript'

export function printInternalExtraInput(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        Object.entries(parsedLogic.extraInput).map(([type, { typeNode, withLogicFunction }]) => {
            if (withLogicFunction) {
                const logicTypeArguments = [...parsedLogic.typeReferencesInLogicInput]
                    .sort()
                    .map((text) => factory.createTypeReferenceNode(factory.createIdentifier(text), undefined))

                return factory.createPropertySignature(
                    undefined,
                    factory.createStringLiteral(type),
                    undefined,

                    factory.createUnionTypeNode([
                        typeNode,
                        factory.createFunctionTypeNode(
                            undefined,
                            [
                                factory.createParameterDeclaration(
                                    undefined,
                                    undefined,
                                    undefined,
                                    factory.createIdentifier('logic'),
                                    undefined,
                                    factory.createTypeReferenceNode(
                                        factory.createIdentifier(parsedLogic.logicTypeName),
                                        logicTypeArguments.length > 0 ? logicTypeArguments : undefined,
                                    ),
                                    undefined,
                                ),
                            ],
                            typeNode,
                        ),
                    ]),
                )
            }

            return factory.createPropertySignature(undefined, factory.createStringLiteral(type), undefined, typeNode)
        }),
    )
}
