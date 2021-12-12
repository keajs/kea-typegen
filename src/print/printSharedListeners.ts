import { factory, SyntaxKind } from 'typescript'
import { ParsedLogic } from '../types'

export function printSharedListeners(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        parsedLogic.sharedListeners.map(({ name, payload, action }) =>
            factory.createPropertySignature(
                undefined,
                factory.createStringLiteral(name),
                undefined,
                factory.createFunctionTypeNode(
                    undefined,
                    [
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('payload'),
                            undefined,
                            payload || factory.createKeywordTypeNode(SyntaxKind.AnyKeyword),
                            undefined,
                        ),
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('breakpoint'),
                            undefined,
                            factory.createTypeReferenceNode(factory.createIdentifier('BreakPointFunction'), undefined),
                            undefined,
                        ),
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('action'),
                            undefined,
                            action,
                            undefined,
                        ),
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('previousState'),
                            undefined,
                            factory.createKeywordTypeNode(SyntaxKind.AnyKeyword),
                            undefined,
                        ),
                    ],
                    factory.createUnionTypeNode([
                        { ...factory.createToken(SyntaxKind.VoidKeyword), _typeNodeBrand: true },
                        factory.createTypeReferenceNode(factory.createIdentifier('Promise'), [
                            { ...factory.createToken(SyntaxKind.VoidKeyword), _typeNodeBrand: true },
                        ]),
                    ]),
                ),
            ),
        ),
    )
}
