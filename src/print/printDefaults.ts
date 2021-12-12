import { factory } from 'typescript'
import { ParsedLogic } from '../types'
import { cleanDuplicateAnyNodes } from '../utils'

export function printDefaults(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        cleanDuplicateAnyNodes(parsedLogic.reducers).map((reducer) => {
            return factory.createPropertySignature(
                undefined,
                factory.createIdentifier(reducer.name),
                undefined,
                reducer.typeNode,
            )
        }),
    )
}
