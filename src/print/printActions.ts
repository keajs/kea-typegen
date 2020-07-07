import {ParsedLogic} from "../types";
import * as ts from "typescript";

export function printActions(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        parsedLogic.actions.map((action) => {
            const returnType = action.signature.getReturnType()
            const parameters = action.signature.getDeclaration().parameters

            return ts.createPropertySignature(
                undefined,
                ts.createIdentifier(action.name),
                undefined,
                ts.createFunctionTypeNode(
                    undefined,
                    parameters,
                    ts.createParenthesizedType(
                        ts.createTypeLiteralNode([
                            ts.createPropertySignature(
                                undefined,
                                ts.createIdentifier('type'),
                                undefined,
                                ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                                undefined,
                            ),
                            ts.createPropertySignature(
                                undefined,
                                ts.createIdentifier('payload'),
                                undefined,
                                parsedLogic.checker.typeToTypeNode(returnType),
                                undefined,
                            ),
                        ]),
                    ),
                ),
                undefined,
            )
        }),
    )
}
