import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function printActions(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        parsedLogic.actions.map(({ name, parameters, returnTypeNode }) => {
            return ts.createPropertySignature(
                undefined,
                ts.createIdentifier(name),
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
                                returnTypeNode,
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
