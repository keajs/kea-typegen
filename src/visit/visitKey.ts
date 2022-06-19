import { ParsedLogic } from '../types'
import { cloneNode } from '@wessberg/ts-clone-node'
import { gatherImports } from '../utils'
import { Expression, Type } from 'typescript'

export function visitKey(parsedLogic: ParsedLogic, type: Type, expression: Expression) {
    const { checker } = parsedLogic
    const typeNode = checker.typeToTypeNode(type, undefined, undefined)
    gatherImports(typeNode, checker, parsedLogic)

    parsedLogic.keyType = cloneNode(typeNode)
}
