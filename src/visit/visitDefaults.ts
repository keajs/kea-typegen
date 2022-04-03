import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { getAndGatherTypeNodeForDefaultValue } from '../utils'
import { Expression, Type } from 'typescript'

export function visitDefaults(parsedLogic: ParsedLogic, type: Type, expression: Expression) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

        const typeNode = getAndGatherTypeNodeForDefaultValue(value, checker, parsedLogic)
        if (typeNode) {
            parsedLogic.reducers.push({ name, typeNode })
        }
    }
}
