import * as ts from 'typescript'
import { AppOptions, ParsedLogic } from '../types'
import { getActionTypeCreator } from '../utils'

export function printActions(parsedLogic: ParsedLogic, appOptions?: AppOptions) {
    const getActionType = getActionTypeCreator(parsedLogic)

    return ts.createTypeLiteralNode(
        parsedLogic.actions.map(({ name, parameters, returnTypeNode }) => {
            return ts.createPropertySignature(
                undefined,
                ts.createIdentifier(name),
                undefined,
                ts.createFunctionTypeNode(
                    undefined,
                    parameters,
                    // Please someone tell me what must I do here?
                    // Just ts.createToken(void), as suggested by ts-ast-viewer.com doesn't work
                    { ...ts.createToken(ts.SyntaxKind.VoidKeyword), _typeNodeBrand: true }
                ),
                undefined,
            )
        }),
    )
}
