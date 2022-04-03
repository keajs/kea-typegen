import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { extractImportedActions, getAndGatherTypeNodeForDefaultValue } from '../utils'
import { cloneNode } from '@wessberg/ts-clone-node'

export function visitReducers(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

        if (value) {
            let extraActions = {}
            let typeNode

            if (ts.isArrayLiteralExpression(value)) {
                let defaultValue = value.elements[0]
                typeNode = getAndGatherTypeNodeForDefaultValue(defaultValue, checker, parsedLogic)
                if (ts.isFunctionTypeNode(typeNode)) {
                    typeNode = typeNode.type
                }

                if (value.elements.length > 1) {
                    const actionObjects = value.elements[value.elements.length - 1]
                    extraActions = extractImportedActions(actionObjects, checker, parsedLogic)
                }
            } else if (ts.isObjectLiteralExpression(value)) {
                typeNode = ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
            }

            parsedLogic.reducers.push({ name, typeNode })

            if (Object.keys(extraActions).length > 0) {
                Object.assign(parsedLogic.extraActions, extraActions)
            }
        }
    }
}
