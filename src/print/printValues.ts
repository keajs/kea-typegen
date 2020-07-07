import {ParsedLogic} from "../types";
import * as ts from "typescript";

export function printValues(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(parsedLogic.reducers.concat(parsedLogic.selectors).map((reducer) => {
        return ts.createPropertySignature(
            undefined,
            ts.createIdentifier(reducer.name),
            undefined,
            reducer.typeNode,
            undefined,
        )
    }))
}
