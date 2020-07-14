import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function printSelectorTypeHelp(parsedLogic: ParsedLogic) {
    return ts.createTypeLiteralNode(
        parsedLogic.selectors
            .filter((s) => s.functionTypes.length > 0)
            .map((selector) => {
                return ts.createPropertySignature(
                    undefined,
                    ts.createIdentifier(selector.name),
                    undefined,

                    ts.createFunctionTypeNode(
                        undefined,
                        selector.functionTypes.map((functionType, index) =>
                            ts.createParameter(
                                undefined,
                                undefined,
                                undefined,
                                ts.createIdentifier(functionType.name),
                                ts.createToken(ts.SyntaxKind.QuestionToken),
                                functionType.type,
                                undefined,
                            ),
                        ),
                        ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                    ),
                    undefined,
                )
            }),
    )
}
