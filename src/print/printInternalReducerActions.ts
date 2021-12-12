import { ParsedLogic } from '../types'
import { factory } from 'typescript'

export function printInternalReducerActions(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        Object.entries(parsedLogic.extraActions).map(([type, typeNode]) => {
            return factory.createPropertySignature(undefined, factory.createStringLiteral(type), undefined, typeNode)
        }),
    )
}
