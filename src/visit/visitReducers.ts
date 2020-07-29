import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { getTypeNodeForDefaultValue } from '../utils'
import { cloneNode } from '@wessberg/ts-clone-node'

function extractImportedActions(actionObjects: ts.Expression | ts.ObjectLiteralExpression, checker: ts.TypeChecker) {
    let extraActions = {}

    if (ts.isObjectLiteralExpression(actionObjects)) {
        // actionObjects =  { [githubLogic.actionTypes.setRepositories]: () => ... }
        for (const property of actionObjects.properties) {
            // property.name ==> [githubLogic.actionTypes.setRepositories]
            if (ts.isComputedPropertyName(property.name)) {
                let propertyExpression = property.name.expression

                if (ts.isPropertyAccessExpression(propertyExpression)) {
                    const {name, expression} = propertyExpression
                    const actionName = name.escapedText

                    const nameSymbol = checker.getSymbolAtLocation(property.name)
                    const actionType = nameSymbol.escapedName as string

                    if (ts.isPropertyAccessExpression(expression)) {
                        // expression.expression ==> githubLogic.actionTypes
                        // expression.name ==> setRepositories

                        const symbol = checker.getSymbolAtLocation(expression.expression)
                        const symbolType = checker.getTypeOfSymbolAtLocation(symbol, expression.expression)

                        const actionCreatorsProperty = symbolType.getProperties().find((p) => p.escapedName === 'actionCreators')
                        const actionCreators = actionCreatorsProperty?.valueDeclaration

                        if (actionCreators && ts.isPropertySignature(actionCreators) && ts.isTypeLiteralNode(actionCreators.type)) {
                            const actionCreator = actionCreators.type.members.find(
                                (m) => (m.name as ts.Identifier)?.escapedText === actionName,
                            )

                            if (actionCreator && ts.isPropertySignature(actionCreator) && ts.isFunctionTypeNode(actionCreator.type)) {
                                extraActions[actionType] = cloneNode(actionCreator.type) //payload
                            }
                        }
                    }
                }
            }
        }
    }
    return extraActions
}

export function visitReducers(type: ts.Type, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

        if (value) {
            let extraActions = {}
            let typeNode
            if (ts.isArrayLiteralExpression(value)) {
                const defaultValue = value.elements[0]

                const actionObjects = value.elements[value.elements.length - 1]
                extraActions = extractImportedActions(actionObjects, checker)
                typeNode = getTypeNodeForDefaultValue(defaultValue, checker)
            } else if (ts.isObjectLiteralExpression(value)) {
                typeNode = ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
            }
            parsedLogic.reducers.push({ name, typeNode, extraActions })
        }
    }
}
