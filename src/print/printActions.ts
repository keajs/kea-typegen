import { factory, SyntaxKind } from 'typescript'
import { AppOptions, ParsedLogic } from '../types'
import { getActionTypeCreator } from '../utils'

export function printActions(parsedLogic: ParsedLogic, appOptions?: AppOptions) {
    const getActionType = getActionTypeCreator(parsedLogic)

    return factory.createTypeLiteralNode(
        parsedLogic.actions.map(({ name, parameters }) => {
            return factory.createPropertySignature(
                undefined,
                factory.createIdentifier(name),
                undefined,
                factory.createFunctionTypeNode(undefined, parameters, factory.createToken(SyntaxKind.VoidKeyword)),
            )
        }),
    )
}
