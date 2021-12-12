import { factory, SyntaxKind } from 'typescript'
import { ParsedLogic } from '../types'

export function printEvents(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        Object.keys(parsedLogic.events).map((name) => {
            return factory.createPropertySignature(
                undefined,
                factory.createIdentifier(name),
                undefined,
                factory.createFunctionTypeNode(undefined, [], {
                    ...factory.createToken(SyntaxKind.VoidKeyword),
                    _typeNodeBrand: true,
                }),
            )
        }),
    )
}
