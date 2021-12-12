import { factory } from 'typescript'
import { AppOptions, ParsedLogic } from '../types'
import { getActionTypeCreator } from '../utils'

export function printActionCreators(parsedLogic: ParsedLogic, appOptions?: AppOptions) {
    const getActionType = getActionTypeCreator(parsedLogic)

    return factory.createTypeLiteralNode(
        parsedLogic.actions.map(({ name, parameters, returnTypeNode }) => {
            return factory.createPropertySignature(
                undefined,
                factory.createIdentifier(name),
                undefined,
                factory.createFunctionTypeNode(
                    undefined,
                    parameters,
                    factory.createParenthesizedType(
                        factory.createTypeLiteralNode([
                            factory.createPropertySignature(
                                undefined,
                                factory.createIdentifier('type'),
                                undefined,
                                factory.createLiteralTypeNode(factory.createStringLiteral(getActionType(name))),
                            ),
                            factory.createPropertySignature(
                                undefined,
                                factory.createIdentifier('payload'),
                                undefined,
                                returnTypeNode,
                            ),
                        ]),
                    ),
                ),
            )
        }),
    )
}
