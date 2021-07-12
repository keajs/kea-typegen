import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function printInternalExtraInput(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        Object.entries(parsedLogic.extraInput).map(([type, { typeNode, withLogicFunction }]) => {
            if (withLogicFunction) {
                const logicTypeArguments = [...parsedLogic.typeReferencesInLogicInput]
                    .sort()
                    .map((text) => ts.createTypeReferenceNode(ts.createIdentifier(text), undefined))

                return ts.createPropertySignature(
                    undefined,
                    ts.createStringLiteral(type),
                    undefined,

                    ts.createUnionTypeNode([
                        typeNode,
                        ts.createFunctionTypeNode(
                            undefined,
                            [
                                ts.createParameter(
                                    undefined,
                                    undefined,
                                    undefined,
                                    ts.createIdentifier('logic'),
                                    undefined,
                                    ts.createTypeReferenceNode(
                                        ts.createIdentifier(parsedLogic.logicTypeName),
                                        logicTypeArguments.length > 0 ? logicTypeArguments : undefined,
                                    ),
                                    undefined,
                                ),
                            ],
                            typeNode,
                        ),
                    ]),
                    undefined,
                )
            }

            return ts.createPropertySignature(undefined, ts.createStringLiteral(type), undefined, typeNode, undefined)
        }),
    )
}
