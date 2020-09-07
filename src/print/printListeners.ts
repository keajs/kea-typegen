import * as ts from 'typescript'
import { ParsedLogic } from '../types'

export function printListeners(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        parsedLogic.listeners.map(({ name, payload, action }) =>
            ts.createPropertySignature(
                undefined,
                ts.createStringLiteral(name),
                undefined,
                ts.createArrayTypeNode(
                    ts.createParenthesizedType(
                        ts.createFunctionTypeNode(
                            undefined,
                            [
                                ts.createParameter(
                                    undefined,
                                    undefined,
                                    undefined,
                                    ts.createIdentifier('action'),
                                    undefined,
                                    action,
                                    undefined,
                                ),
                                ts.createParameter(
                                    undefined,
                                    undefined,
                                    undefined,
                                    ts.createIdentifier('previousState'),
                                    undefined,
                                    ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                                    undefined,
                                ),
                            ],
                            ts.createUnionTypeNode([
                                { ...ts.createToken(ts.SyntaxKind.VoidKeyword), _typeNodeBrand: true },
                                ts.createTypeReferenceNode(ts.createIdentifier('Promise'), [
                                    { ...ts.createToken(ts.SyntaxKind.VoidKeyword), _typeNodeBrand: true },
                                ]),
                            ]),
                        ),
                    ),
                ),
                undefined,
            ),
        ),
    )
}
