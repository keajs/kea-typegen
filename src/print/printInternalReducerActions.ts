import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import {combineExtraActions} from "../utils";

export function printInternalReducerActions(parsedLogic: ParsedLogic) {
    const extraActions = combineExtraActions(parsedLogic.reducers)

    const parameters = []

    return ts.createTypeLiteralNode(
        Object.entries(extraActions)
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
