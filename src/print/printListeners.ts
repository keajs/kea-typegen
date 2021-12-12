import { factory, SyntaxKind } from 'typescript'
import { ParsedLogic } from '../types'

export function printListeners(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        parsedLogic.listeners.map(({ name, payload, action }) =>
            factory.createPropertySignature(
                undefined,
                factory.createStringLiteral(name),
                undefined,
                factory.createArrayTypeNode(
                    factory.createParenthesizedType(
                        factory.createFunctionTypeNode(
                            undefined,
                            [
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
            ),
        ),
    )
}
