import { ParsedLogic } from '../types'
import { cloneNodeSorted, gatherImports } from '../utils'
import { Expression, Type } from 'typescript'

export function visitKey(parsedLogic: ParsedLogic, type: Type, expression: Expression) {
    const { checker } = parsedLogic
    const typeNode = checker.typeToTypeNode(type, undefined, undefined)
    gatherImports(typeNode, checker, parsedLogic)

    parsedLogic.keyType = cloneNodeSorted(typeNode)
}
