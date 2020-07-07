import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function printActions(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        parsedLogic.actions.map((action) => {
            let returnTypeNode
            let parameters: ts.NodeArray<ts.ParameterDeclaration> | undefined = undefined

            if (ts.isFunctionTypeNode(action.typeNode)) {
                parameters = action.signature.getDeclaration().parameters
                returnTypeNode = parsedLogic.checker.typeToTypeNode(action.signature.getReturnType())
            } else {
                returnTypeNode = ts.createTypeLiteralNode([
                    ts.createPropertySignature(
                        undefined,
                        ts.createIdentifier('value'),
                        undefined,
                        action.typeNode,
                        undefined,
                    ),
                ])
            }

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
