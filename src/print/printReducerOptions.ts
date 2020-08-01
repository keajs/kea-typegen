import * as ts from 'typescript'
import { ParsedLogic, ReducerTransform } from '../types'
import { cleanDuplicateAnyNodes } from '../utils'

export function printReducerOptions(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        cleanDuplicateAnyNodes(parsedLogic.reducers)
            .filter((r: ReducerTransform) => r.reducerOptions)
            .map((reducer: ReducerTransform) => {
                if (reducer.reducerOptions) {
                    return ts.createPropertySignature(
                        undefined,
                        ts.createIdentifier(reducer.name),
                        undefined,
                        reducer.reducerOptions,
                        undefined,
                    )
                }
            })
            .filter((a) => a),
    )
}
