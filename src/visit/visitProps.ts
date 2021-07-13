import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { getAndGatherTypeNodeForDefaultValue } from '../utils'

export function visitProps(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic
    parsedLogic.propsType = getAndGatherTypeNodeForDefaultValue(inputProperty.initializer, checker, parsedLogic)
}
