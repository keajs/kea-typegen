import { Plugin } from '../../src/types'
import * as ts from 'typescript'

export default {
    visitKeaProperty({ name, parsedLogic }) {
        if (name === 'forms') {
            // add action "ranFormsPlugin" to parsedLogic
            parsedLogic.actions.push({
                name: 'ranFormsPlugin',
                parameters: [],
                returnTypeNode: ts.createTypeLiteralNode([
                    ts.createPropertySignature(undefined, ts.createIdentifier('value'), undefined, ts.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword), undefined),
                ]),
            })
        }
    },
} as Plugin
