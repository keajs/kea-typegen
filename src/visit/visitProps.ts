import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'

export function visitProps(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic
    const typeNode = checker.typeToTypeNode(type, undefined, undefined)

    if (ts.isTypeLiteralNode(typeNode) && type.getProperties().length > 0) {
        parsedLogic.propsType = cloneNode(typeNode)
    }
}
