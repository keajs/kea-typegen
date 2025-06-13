import { Plugin } from '../../src/types'
import * as ts from 'typescript'
import { factory } from 'typescript'

export default {
    visitKeaProperty({ name, parsedLogic, type }) {
        if (name === 'inline') {
            parsedLogic.importFromKeaInLogicType.add('NotReallyHereButImportedBecausePlugin')

            // add action "submitForm" to parsedLogic
            parsedLogic.actions.push({
                name: 'inlineAction',
                parameters: [],
                returnTypeNode: factory.createTypeLiteralNode([
                    factory.createPropertySignature(
                        undefined,
                        factory.createIdentifier('value'),
                        undefined,
                        factory.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword),
                    ),
                ]),
            })

            // add reducer "form" to parsedLogic
            parsedLogic.reducers.push({
                name: 'inlineReducer',
                typeNode: factory.createTypeLiteralNode([
                    factory.createPropertySignature(
                        undefined,
                        factory.createIdentifier('asd'),
                        undefined,
                        factory.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword),
                    ),
                ]),
            })
        }
    },
} as Plugin
