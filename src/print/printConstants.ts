import * as ts from 'typescript'
import { ParsedLogic } from '../types'

export function printConstants(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        parsedLogic.constants.map((constant) =>
            ts.createPropertySignature(
                undefined,
                ts.createStringLiteral(constant),
                undefined,
                ts.createLiteralTypeNode(ts.createStringLiteral(constant)),
                undefined,
            ),
        ),
    )
}
