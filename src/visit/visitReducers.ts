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
            let reducerOptions

            if (ts.isArrayLiteralExpression(value)) {
                let defaultValue = value.elements[0]
                typeNode = getAndGatherTypeNodeForDefaultValue(defaultValue, checker, parsedLogic)
                if (ts.isFunctionTypeNode(typeNode)) {
                    typeNode = typeNode.type
                }

                const actionObjects = value.elements[value.elements.length - 1]
                extraActions = extractImportedActions(actionObjects, checker, parsedLogic)

                if (value.elements.length > 2) {
                    const options = value.elements[value.elements.length - 2]
                    if (ts.isObjectLiteralExpression(options)) {
                        reducerOptions = cloneNode(
                            checker.typeToTypeNode(checker.getTypeAtLocation(options), undefined, undefined),
                        )
                    }
                }
            } else if (ts.isObjectLiteralExpression(value)) {
                typeNode = ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
            }

            parsedLogic.reducers.push({ name, typeNode, reducerOptions })

            if (Object.keys(extraActions).length > 0) {
                Object.assign(parsedLogic.extraActions, extraActions)
            }
        }
    }
}
