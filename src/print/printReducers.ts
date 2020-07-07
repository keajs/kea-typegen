import {ParsedLogic} from "../types";
import * as ts from "typescript";

export function printReducers(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(parsedLogic.reducers.map((reducer) => {
        return ts.createPropertySignature(
            undefined,
            ts.createIdentifier(reducer.name),
            undefined,
            ts.createFunctionTypeNode(
                undefined,
                [
                    ts.createParameter(
                        undefined,
                        undefined,
                        undefined,
                        ts.createIdentifier('state'),
                        undefined,
                        reducer.typeNode,
                        undefined,
                    ),
                    ts.createParameter(
                        undefined,
                        undefined,
                        undefined,
                        ts.createIdentifier('action'),
                        undefined,
                        ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        undefined,
                    ),
                    ts.createParameter(
                        undefined,
                        undefined,
                        undefined,
                        ts.createIdentifier('fullState'),
                        undefined,
                        ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        undefined,
                    ),
                ],
                reducer.typeNode,
            ),
            undefined,
        )
    }))
}
