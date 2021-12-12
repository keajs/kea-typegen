import { factory, SyntaxKind } from 'typescript'
import { ParsedLogic } from '../types'

export function printKey(parsedLogic: ParsedLogic) {
    return parsedLogic.keyType || factory.createKeywordTypeNode(SyntaxKind.UndefinedKeyword)
}
