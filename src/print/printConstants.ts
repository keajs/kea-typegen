import { factory } from 'typescript'
import { ParsedLogic } from '../types'

export function printConstants(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        parsedLogic.constants.map((constant) =>
            factory.createPropertySignature(
                undefined,
                factory.createStringLiteral(constant),
                undefined,
                factory.createLiteralTypeNode(factory.createStringLiteral(constant)),
            ),
        ),
    )
}
