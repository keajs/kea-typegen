import * as ts from 'typescript'
import { ParsedLogic } from '../types'

export function printKey(parsedLogic: ParsedLogic) {
    return parsedLogic.keyType || ts.createKeywordTypeNode(ts.SyntaxKind.UndefinedKeyword)
}
