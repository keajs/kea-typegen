import * as ts from 'typescript'
import { ParsedLogic } from '../types'

export function printProps(parsedLogic: ParsedLogic) {
    return parsedLogic.propsType || ts.createTypeReferenceNode(
        ts.createIdentifier("Record"),
        [
            ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
            ts.createKeywordTypeNode(ts.SyntaxKind.UnknownKeyword)
        ]
    )
}
