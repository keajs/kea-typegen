import { factory, SyntaxKind } from 'typescript'
import { ParsedLogic } from '../types'
import { cleanDuplicateAnyNodes } from '../utils'

export function printSelector(parsedLogic: ParsedLogic) {
    return factory.createFunctionTypeNode(
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
        ],
        factory.createTypeLiteralNode(
            cleanDuplicateAnyNodes(parsedLogic.reducers).map((reducer) =>
                factory.createPropertySignature(
                    undefined,
                    factory.createIdentifier(reducer.name),
                    undefined,
                    reducer.typeNode,
                ),
            ),
        ),
    )
}
