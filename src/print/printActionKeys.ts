import { factory } from 'typescript'
import { AppOptions, ParsedLogic } from '../types'
import { getActionTypeCreator } from '../utils'

export function printActionKeys(parsedLogic: ParsedLogic, appOptions?: AppOptions) {
    const getActionType = getActionTypeCreator(parsedLogic)

    return factory.createTypeLiteralNode(
        parsedLogic.actions.map(({ name, parameters, returnTypeNode }) => {
            return factory.createPropertySignature(
                undefined,
                factory.createStringLiteral(getActionType(name)),
                undefined,
                factory.createLiteralTypeNode(factory.createStringLiteral(name)),
            )
        }),
    )
}
