import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'
import { NodeBuilderFlags } from 'typescript'

export function visitKey(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic
    const typeNode = checker.typeToTypeNode(type, inputProperty, NodeBuilderFlags.NoTruncation)

    parsedLogic.keyType = cloneNode(typeNode)
}
