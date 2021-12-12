import { factory, SyntaxKind } from 'typescript'
import { ParsedLogic } from '../types'
import { cleanDuplicateAnyNodes } from '../utils'

export function printSelectors(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        cleanDuplicateAnyNodes(parsedLogic.reducers.concat(parsedLogic.selectors)).map((reducer) => {
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
                            factory.createKeywordTypeNode(SyntaxKind.AnyKeyword),
                            undefined,
                        ),
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('props'),
                            factory.createToken(SyntaxKind.QuestionToken),
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
