import { Plugin } from '../../src/types'
import * as ts from 'typescript'

export default {
    visitKeaProperty({ name, parsedLogic, node, getTypeNodeForNode, prepareForPrint }) {
        if (name === 'inline') {
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
                name: 'inlineReducer',
                typeNode:
                    // the default given to us... or
                    typeNode ||
                    // ... Record<string, any>
                    ts.createTypeReferenceNode(ts.createIdentifier('Record'), [
                        ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                        ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                    ]),
            })

            // add action "submitForm" to parsedLogic
            parsedLogic.actions.push({
                name: 'inlineAction',
                parameters: [],
                returnTypeNode: ts.createTypeLiteralNode([
                    ts.createPropertySignature(
                        undefined,
                        ts.createIdentifier('value'),
                        undefined,
                        ts.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword),
                        undefined,
                    ),
                ]),
            })
        }
    },
} as Plugin
