import * as ts from 'typescript'
import { ParsedLogic } from '../types'
import {cleanDuplicateAnyNodes} from "../utils";

export function printValues(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        cleanDuplicateAnyNodes(parsedLogic.reducers.concat(parsedLogic.selectors)).map((reducer) => {
            return ts.createPropertySignature(
                undefined,
                ts.createIdentifier(reducer.name),
                undefined,
                reducer.typeNode,
                undefined,
            )
        }),
    )
}
