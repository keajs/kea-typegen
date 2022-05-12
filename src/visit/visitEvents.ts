import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { Expression, Type } from 'typescript'

export function visitEvents(parsedLogic: ParsedLogic, type: Type, expression: Expression) {
    for (const property of type.getProperties()) {
        const name = property.getName()
        parsedLogic.events[name] = true
    }
}
