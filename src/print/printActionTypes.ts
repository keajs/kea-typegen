import { factory } from 'typescript'
import { AppOptions, ParsedLogic } from '../types'
import { getActionTypeCreator } from '../utils'

export function printActionTypes(parsedLogic: ParsedLogic, appOptions?: AppOptions) {
    const getActionType = getActionTypeCreator(parsedLogic)

    return factory.createTypeLiteralNode(
        parsedLogic.actions.map(({ name }) => {
            return factory.createPropertySignature(
                undefined,
                factory.createIdentifier(name),
                undefined,
                factory.createLiteralTypeNode(factory.createStringLiteral(getActionType(name))),
            )
        }),
    )
}
