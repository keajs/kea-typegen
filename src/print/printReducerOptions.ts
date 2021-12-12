import { factory } from 'typescript'
import { ParsedLogic, ReducerTransform } from '../types'
import { cleanDuplicateAnyNodes } from '../utils'

export function printReducerOptions(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        cleanDuplicateAnyNodes(parsedLogic.reducers)
            .filter((r: ReducerTransform) => r.reducerOptions)
            .map((reducer: ReducerTransform) => {
                if (reducer.reducerOptions) {
                    return factory.createPropertySignature(
                        undefined,
                        factory.createIdentifier(reducer.name),
                        undefined,
                        reducer.reducerOptions,
                    )
                }
            })
            .filter((a) => a),
    )
}
