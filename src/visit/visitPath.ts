import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function visitPath(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    let objectLiteral = inputProperty.initializer
    let path

    if (ts.isFunctionLike(objectLiteral)) {
        objectLiteral = (objectLiteral as any).body
    }

    if (ts.isParenthesizedExpression(objectLiteral)) {
        objectLiteral = objectLiteral.expression
    }

    if (ts.isArrayLiteralExpression(objectLiteral)) {
        path = objectLiteral.elements.map(element => {
            if (ts.isStringLiteral(element)) {
                return element.text
            } else {
                return '*'
            }
        })
    }

    if (path) {
        parsedLogic.path = path
        parsedLogic.pathString = path.join('.')
    }
}
