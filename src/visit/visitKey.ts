import { ParsedLogic } from '../types'
import { cloneNodeSorted, gatherImports } from '../utils'
import {Expression, NodeBuilderFlags, Type} from 'typescript'

export function visitKey(parsedLogic: ParsedLogic, type: Type, expression: Expression) {
    const { checker } = parsedLogic
    const typeNode = checker.typeToTypeNode(type, undefined, NodeBuilderFlags.NoTruncation)
    gatherImports(typeNode, checker, parsedLogic)

    parsedLogic.keyType = cloneNodeSorted(typeNode)
}
