import { ParsedLogic } from '../types'
import { factory } from 'typescript'

export function printInternalSelectorTypes(parsedLogic: ParsedLogic) {
    return factory.createTypeLiteralNode(
        parsedLogic.selectors
            .filter((s) => s.functionTypes.length > 0)
            .map((selector) => {
                return factory.createPropertySignature(
                    undefined,
                    factory.createIdentifier(selector.name),
                    undefined,

                    factory.createFunctionTypeNode(
                        undefined,
                        selector.functionTypes.map((functionType, index) =>
                            factory.createParameterDeclaration(
                                undefined,
                                undefined,
                                undefined,
                                factory.createIdentifier(functionType.name),
                                undefined,
                                functionType.type,
                                undefined,
                            ),
                        ),
                        selector.typeNode,
                    ),
                )
            }),
    )
}
