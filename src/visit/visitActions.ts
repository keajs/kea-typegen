import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { NodeBuilderFlags } from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'
import { gatherImports } from '../utils'

export function visitActions(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic
    const properties = checker.getPropertiesOfType(type)

    for (const property of properties) {
        const type = checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration!)
        const name = property.getName()

        let returnTypeNode
        let parameters

        if (!ts.isPropertyAssignment(property.valueDeclaration)) {
            continue
        }
        const { initializer } = property.valueDeclaration

        if (ts.isFunctionLike(initializer)) {
            // action is a function action: () => ({ ... })
            parameters = initializer.parameters.map((param) => {
                if (param.type) {
                    gatherImports(param.type, checker, parsedLogic)
                }
                return ts.createParameter(
                    undefined,
                    undefined,
                    undefined,
                    ts.createIdentifier(param.name.getText()),
                    param.initializer || param.questionToken ? ts.createToken(ts.SyntaxKind.QuestionToken) : undefined,
                    param.type ? cloneNode(param.type) : ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                    undefined,
                )
            })

            // first try the specified type (action: (): Type => {...})
            returnTypeNode = initializer.type

            if (!returnTypeNode) {
                // if not found, use the TS compiler to detect it
                const signature = type.getCallSignatures()[0]
                const sigReturnType = signature.getReturnType()
                const sigReturnTypeNode = checker.typeToTypeNode(
                    sigReturnType,
                    undefined,
                    NodeBuilderFlags.NoTruncation,
                )
                returnTypeNode = sigReturnTypeNode
            }

            gatherImports(returnTypeNode, checker, parsedLogic)
            returnTypeNode = cloneNode(returnTypeNode)
        } else {
            // action is a value (action: true)
            const typeNode = checker.typeToTypeNode(type, undefined, undefined)
            gatherImports(typeNode, checker, parsedLogic)
            returnTypeNode = ts.createTypeLiteralNode([
                ts.createPropertySignature(undefined, ts.createIdentifier('value'), undefined, typeNode, undefined),
            ])
        }
        parsedLogic.actions.push({ name, parameters, returnTypeNode })
    }
}
