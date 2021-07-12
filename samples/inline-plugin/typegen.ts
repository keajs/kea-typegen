import { Plugin } from '../../src/types'
import * as ts from 'typescript'

export default {
    visitKeaProperty({ name, parsedLogic, type }) {
        if (name === 'inline') {
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

            // add reducer "form" to parsedLogic
            parsedLogic.reducers.push({
                name: 'inlineReducer',
                typeNode: ts.createTypeLiteralNode([
                    ts.createPropertySignature(
                        undefined,
                        ts.createIdentifier('asd'),
                        undefined,
                        ts.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword),
                    ),
                ]),
            })
        }
    },
} as Plugin