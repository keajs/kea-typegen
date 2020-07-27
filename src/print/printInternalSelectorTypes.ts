import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function printInternalSelectorTypes(parsedLogic: ParsedLogic) {
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
                                undefined,
                                functionType.type,
                                undefined,
                            ),
                        ),
                        selector.typeNode,
                    ),
                    undefined,
                )
            }),
    )
}
