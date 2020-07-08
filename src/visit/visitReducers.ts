import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import {getTypeNodeForDefaultValue} from "../utils";

export function visitReducers(type: ts.Type, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

        let typeNode
        if (ts.isArrayLiteralExpression(value)) {
            const defaultValue = value.elements[0]
            typeNode = getTypeNodeForDefaultValue(defaultValue, checker)
        } else if (ts.isObjectLiteralExpression(value)) {
            typeNode = ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
        }
        parsedLogic.reducers.push({ name, typeNode })
    }
}
