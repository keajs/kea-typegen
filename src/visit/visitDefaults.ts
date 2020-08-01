import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { getTypeNodeForDefaultValue } from '../utils'

export function visitDefaults(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

        const typeNode = getTypeNodeForDefaultValue(value, checker)
        if (typeNode) {
            parsedLogic.reducers.push({ name, typeNode })
        }
    }
}
