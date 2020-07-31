import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { extractImportedActions } from '../utils'

export function visitListeners(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    let objectLiteral = inputProperty.initializer
    let extraActions

    if (ts.isFunctionLike(objectLiteral)) {
        objectLiteral = (objectLiteral as any).body
    }

    if (ts.isParenthesizedExpression(objectLiteral)) {
        objectLiteral = objectLiteral.expression
    }

    if (ts.isObjectLiteralExpression(objectLiteral)) {
        extraActions = extractImportedActions(objectLiteral, checker)
    }

    if (extraActions) {
        Object.assign(parsedLogic.extraActions, extraActions)
    }
}
