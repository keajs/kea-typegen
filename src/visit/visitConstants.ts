import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function visitConstants(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    let objectLiteral = inputProperty.initializer

    if (ts.isFunctionLike(objectLiteral)) {
        objectLiteral = (objectLiteral as any).body
    }

    if (ts.isParenthesizedExpression(objectLiteral)) {
        objectLiteral = objectLiteral.expression
    }

    if (ts.isArrayLiteralExpression(objectLiteral)) {
        parsedLogic.constants = objectLiteral.elements.filter(ts.isStringLiteral).map(e => e.text)
    }
}
