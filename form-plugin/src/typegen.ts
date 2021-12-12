import { Plugin } from '../../src/types'
import * as ts from 'typescript'

export default {
    visitKeaProperty({ name, parsedLogic, node, getTypeNodeForNode, prepareForPrint }) {
        if (name === 'form') {
            let typeNode: ts.TypeNode

            // extract `() => ({})` to just `{}`
            if (
                ts.isArrowFunction(node) &&
                ts.isParenthesizedExpression(node.body) &&
                ts.isObjectLiteralExpression(node.body.expression)
            ) {
                node = node.body.expression
            }

            // get type of `default` and prepare it for printing
            if (ts.isObjectLiteralExpression(node)) {
                const defaultProp = node.properties.find((prop) => prop.name.getText() === 'default')
                const defaultTypeNode = getTypeNodeForNode(defaultProp)
                // this gathers type references for auto-import and clones the node for accurate printing
                typeNode = prepareForPrint(defaultTypeNode)
            }

            // add reducer with this default type
            parsedLogic.reducers.push({
                name: 'form',
                typeNode:
                    // the default given to us... or
                    typeNode ||
                    // ... Record<string, any>
                    ts.factory.createTypeReferenceNode(ts.factory.createIdentifier('Record'), [
                        ts.factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                        ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                    ]),
            })

            // add extra type for logic input
            parsedLogic.extraInput['form'] = {
                // adds support for both { inline: (logic) => ({}) } and { inline: {} }
                withLogicFunction: true,
                // type applied in LogicInput
                typeNode: ts.factory.createTypeLiteralNode([
                    // default?: Record<string, any>
                    ts.factory.createPropertySignature(
                        undefined,
                        ts.factory.createIdentifier('default'),
                        ts.factory.createToken(ts.SyntaxKind.QuestionToken),
                        ts.factory.createTypeReferenceNode(ts.factory.createIdentifier('Record'), [
                            ts.factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                            ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        ]),
                    ),
                    // submit?: (form: $typeNode || Record<string, any>) => void
                    ts.factory.createPropertySignature(
                        undefined,
                        ts.factory.createIdentifier('submit'),
                        ts.factory.createToken(ts.SyntaxKind.QuestionToken),
                        ts.factory.createFunctionTypeNode(
                            undefined,
                            [
                                ts.factory.createParameterDeclaration(
                                    undefined,
                                    undefined,
                                    undefined,
                                    ts.factory.createIdentifier('form'),
                                    undefined,
                                    typeNode ||
                                        ts.factory.createTypeReferenceNode(ts.factory.createIdentifier('Record'), [
                                            ts.factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                                            ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                                        ]),
                                    undefined,
                                ),
                            ],
                            ts.factory.createKeywordTypeNode(ts.SyntaxKind.VoidKeyword),
                        ),
                    ),
                ]),
            }

            // add action "submitForm" to parsedLogic
            parsedLogic.actions.push({
                name: 'submitForm',
                parameters: [],
                returnTypeNode: ts.factory.createTypeLiteralNode([
                    ts.factory.createPropertySignature(
                        undefined,
                        ts.factory.createIdentifier('value'),
                        undefined,
                        ts.factory.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword),
                    ),
                ]),
            })
        }
    },
} as Plugin
