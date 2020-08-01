import * as ts from 'typescript'
import { ParsedLogic } from '../types'

export function printSharedListeners(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        parsedLogic.sharedListeners.map(({ name, typeNode }) =>
            ts.createPropertySignature(
                undefined,
                ts.createIdentifier(name),
                undefined,
                ts.createFunctionTypeNode(
                    undefined,
                    [
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier('payload'),
                            undefined,
                            typeNode || ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                            undefined,
                        ),
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier('breakpoint'),
                            undefined,
                            ts.createTypeReferenceNode(ts.createIdentifier('BreakPointFunction'), undefined),
                            undefined,
                        ),
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier('action'),
                            undefined,
                            ts.createTypeLiteralNode([
                                ts.createPropertySignature(
                                    undefined,
                                    ts.createIdentifier("type"),
                                    undefined,
                                    ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                                    undefined
                                ),
                                ts.createPropertySignature(
                                    undefined,
                                    ts.createIdentifier("payload"),
                                    undefined,
                                    typeNode || ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                                    // ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                                    undefined
                                )
                            ])
                            ,
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
                undefined,
            ),
        ),
    )
}
