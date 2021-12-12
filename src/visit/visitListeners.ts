import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { extractImportedActions, getActionTypeCreator } from '../utils'

export function visitListeners(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const getActionType = getActionTypeCreator(parsedLogic)
    const { checker } = parsedLogic
    let extraActions = {}

    let objectLiteral = inputProperty.initializer

    if (ts.isFunctionLike(objectLiteral)) {
        objectLiteral = (objectLiteral as any).body
    }

    if (ts.isParenthesizedExpression(objectLiteral)) {
        objectLiteral = objectLiteral.expression
    }

    if (ts.isObjectLiteralExpression(objectLiteral)) {
        extraActions = extractImportedActions(objectLiteral, checker, parsedLogic)

        if (extraActions) {
            Object.assign(parsedLogic.extraActions, extraActions)
        }
    }

    for (const property of type.getProperties()) {
        const name = property.getName()

        if (extraActions[name]) {
            const actionCreator = extraActions[name]
            if (actionCreator && ts.isFunctionLike(actionCreator)) {
                const actionReturnType = actionCreator.type
                if (actionReturnType && ts.isTypeLiteralNode(actionReturnType)) {
                    const payload = (actionReturnType.members.find(
                        (m) => (m.name as ts.Identifier)?.escapedText === 'payload',
                    ) as ts.PropertySignature)?.type
                    if (payload) {
                        parsedLogic.listeners.push({ name, action: actionReturnType, payload })
                    }
                }
            }
        } else {
            const action = parsedLogic.actions.find((a) => a.name === name)
            if (action) {
                parsedLogic.listeners.push({
                    name: action.name,
                    payload: action.returnTypeNode,
                    action: ts.factory.createTypeLiteralNode([
                        ts.factory.createPropertySignature(
                            undefined,
                            ts.factory.createIdentifier('type'),
                            undefined,
                            ts.factory.createLiteralTypeNode(
                                ts.factory.createStringLiteral(getActionType(action.name)),
                            ),
                        ),
                        ts.factory.createPropertySignature(
                            undefined,
                            ts.factory.createIdentifier('payload'),
                            undefined,
                            action.returnTypeNode,
                        ),
                    ]),
                })
            }
        }
    }
}
