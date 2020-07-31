import * as ts from 'typescript'
import { AppOptions, ParsedLogic } from '../types'
import { getActionTypeCreator } from '../utils'

export function printActionKeys(parsedLogic: ParsedLogic, appOptions?: AppOptions) {
    const getActionType = getActionTypeCreator(parsedLogic)

    return ts.createTypeLiteralNode(
        parsedLogic.actions.map(({ name, parameters, returnTypeNode }) => {
            return ts.createPropertySignature(
                undefined,
                ts.createStringLiteral(getActionType(name)),
                undefined,
                ts.createLiteralTypeNode(ts.createStringLiteral(name)),
                undefined,
            )
        }),
    )
}
