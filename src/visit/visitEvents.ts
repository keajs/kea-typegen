import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function visitEvents(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    for (const property of type.getProperties()) {
        const name = property.getName()
        parsedLogic.events[name] = true
    }
}
