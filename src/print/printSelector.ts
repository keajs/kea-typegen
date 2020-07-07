import {ParsedLogic} from "../types";
import * as ts from "typescript";

export function printSelector(parsedLogic: ParsedLogic) {
    return ts.createFunctionTypeNode(
        undefined,
        [
            ts.createParameter(
                undefined,
                undefined,
                undefined,
                ts.createIdentifier('state'),
                undefined,
                ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                undefined,
            ),
        ],
        ts.createTypeLiteralNode(
            parsedLogic.reducers.map((reducer) =>
                ts.createPropertySignature(
                    undefined,
                    ts.createIdentifier(reducer.name),
                    undefined,
                    reducer.typeNode,
                    undefined,
                ),
            ),
        ),
    )
}