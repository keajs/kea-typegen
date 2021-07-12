import { Plugin } from '../../src/types'
import * as ts from 'typescript'

export default {
    visitKeaProperty({ name, parsedLogic, type }) {
        if (name === 'forms') {
            // add action "submitForm" to parsedLogic
            parsedLogic.actions.push({
                name: 'submitForm',
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
                name: 'form',
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
