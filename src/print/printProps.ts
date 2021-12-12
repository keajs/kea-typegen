import { factory, SyntaxKind } from 'typescript'
import { ParsedLogic } from '../types'

export function printProps(parsedLogic: ParsedLogic) {
    return (
        parsedLogic.propsType ||
        factory.createTypeReferenceNode(factory.createIdentifier('Record'), [
            factory.createKeywordTypeNode(SyntaxKind.StringKeyword),
            factory.createKeywordTypeNode(SyntaxKind.UnknownKeyword),
        ])
    )
}
