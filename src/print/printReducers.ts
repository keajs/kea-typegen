import { factory, SyntaxKind } from 'typescript'
import { ParsedLogic } from '../types'
import { cleanDuplicateAnyNodes } from '../utils'

export function printReducers(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        cleanDuplicateAnyNodes(parsedLogic.reducers).map((reducer) => {
            return factory.createPropertySignature(
                undefined,
                factory.createIdentifier(reducer.name),
                undefined,
                factory.createFunctionTypeNode(
                    undefined,
                    [
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('state'),
                            undefined,
                            reducer.typeNode,
                            undefined,
                        ),
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('action'),
                            undefined,
                            factory.createKeywordTypeNode(SyntaxKind.AnyKeyword),
                            undefined,
                        ),
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('fullState'),
                            undefined,
                            factory.createKeywordTypeNode(SyntaxKind.AnyKeyword),
                            undefined,
                        ),
                    ],
                    reducer.typeNode,
                ),
            )
        }),
    )
}
