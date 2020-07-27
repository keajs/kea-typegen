import * as ts from 'typescript'
import { ParsedLogic } from '../types'
import { cleanDuplicateAnyNodes } from '../utils'

export function printReducer(parsedLogic: ParsedLogic) {
    return ts.createFunctionTypeNode(
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
                ts.createIdentifier('action'),
                undefined,
                ts.createFunctionTypeNode(undefined, [], ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)),
                undefined,
            ),
            ts.createParameter(
                undefined,
                undefined,
                undefined,
                ts.createIdentifier('fullState'),
                undefined,
                ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                undefined,
            ),
        ],
        ts.createTypeLiteralNode(
            cleanDuplicateAnyNodes(parsedLogic.reducers).map((reducer) =>
                ts.createPropertySignature(
                    undefined,
                    ts.createIdentifier(reducer.name),
                    undefined,
                    reducer.typeNode,
                    undefined,
                ),
            ),
        ),
    )
}
