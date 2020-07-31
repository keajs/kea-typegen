import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function printInternalReducerActions(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        Object.entries(parsedLogic.extraActions)
            .map(([type, typeNode]) => {
                return ts.createPropertySignature(
                    undefined,
                    ts.createStringLiteral(type),
                    undefined,
                    typeNode,
                    undefined,
                )
            }),
    )
}
