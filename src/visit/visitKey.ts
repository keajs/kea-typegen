import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'

export function visitKey(type: ts.Type, inputProperty: ts.Node, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic
    const typeNode = checker.typeToTypeNode(type)

    parsedLogic.keyType = cloneNode(typeNode)
}
