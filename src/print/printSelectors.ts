import * as ts from 'typescript'
import { ParsedLogic } from '../types'
import { cleanDuplicateAnyNodes } from '../utils'

export function printSelectors(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        cleanDuplicateAnyNodes(parsedLogic.reducers.concat(parsedLogic.selectors)).map((reducer) => {
            return ts.createPropertySignature(
                undefined,
                ts.createIdentifier(reducer.name),
                undefined,
                ts.createFunctionTypeNode(
                    undefined,
                    [
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier('state'),
                            undefined,
                            ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                            undefined,
                        ),
                        ts.createParameter(
                            undefined,
                            undefined,
                            undefined,
                            ts.createIdentifier('props'),
                            ts.createToken(ts.SyntaxKind.QuestionToken),
                            ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                            undefined,
                        ),
                    ],
                    reducer.typeNode,
                ),
                undefined,
            )
        }),
    )
}
