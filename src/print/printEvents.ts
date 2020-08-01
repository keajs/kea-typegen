import * as ts from 'typescript'
import { ParsedLogic } from '../types'

export function printEvents(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        Object.keys(parsedLogic.events).map((name) => {
            return ts.createPropertySignature(
                undefined,
                ts.createIdentifier(name),
                undefined,
                ts.createFunctionTypeNode(undefined, [], { ...ts.createToken(ts.SyntaxKind.VoidKeyword), _typeNodeBrand: true }),
                undefined,
            )
        }),
    )
}
