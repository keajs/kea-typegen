import { Plugin } from '../../src/types'
import * as ts from 'typescript'

export default {
    visitKeaProperty({ name, parsedLogic, type }) {
        if (name === 'inline') {
            parsedLogic.importFromKeaInLogicType.add('NotReallyHereButImportedBecausePlugin')

            // add action "submitForm" to parsedLogic
            parsedLogic.actions.push({
                name: 'inlineAction',
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

            // add reducer "form" to parsedLogic
            parsedLogic.reducers.push({
                name: 'inlineReducer',
                typeNode: ts.factory.createTypeLiteralNode([
                    ts.factory.createPropertySignature(
                        undefined,
                        ts.factory.createIdentifier('asd'),
                        undefined,
                        ts.factory.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword),
                    ),
                ]),
            })
        }
    },
} as Plugin
