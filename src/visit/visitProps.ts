import { ParsedLogic } from '../types'
import { getAndGatherTypeNodeForDefaultValue } from '../utils'
import { Expression, Type } from 'typescript'

export function visitProps(parsedLogic: ParsedLogic, type: Type, expression: Expression) {
    const { checker } = parsedLogic
    parsedLogic.propsType = getAndGatherTypeNodeForDefaultValue(expression, checker, parsedLogic)
}
