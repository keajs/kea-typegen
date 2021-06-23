import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'
import { gatherImports } from '../utils'

export function visitKey(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic
    const typeNode = checker.typeToTypeNode(type, undefined, undefined)
    gatherImports(typeNode, checker, parsedLogic)

    parsedLogic.keyType = cloneNode(typeNode)
}
