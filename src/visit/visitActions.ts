import { ParsedLogic } from '../types'
import { factory, isFunctionLike, isPropertyAssignment, PropertyAssignment, SyntaxKind, Type } from 'typescript'
import { NodeBuilderFlags } from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'
import { gatherImports } from '../utils'

export function visitActions(type: Type, inputProperty: PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic
    const properties = checker.getPropertiesOfType(type)

    for (const property of properties) {
        const type = checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration!)
        const name = property.getName()

        let returnTypeNode
        let parameters

        if (!isPropertyAssignment(property.valueDeclaration)) {
            continue
        }
        const { initializer } = property.valueDeclaration

        if (isFunctionLike(initializer)) {
            // action is a function action: () => ({ ... })
            parameters = initializer.parameters.map((param) => {
                if (param.type) {
                    gatherImports(param.type, checker, parsedLogic)
                }
                return factory.createParameterDeclaration(
                    undefined,
                    undefined,
                    undefined,
                    factory.createIdentifier(param.name.getText()),
                    param.initializer || param.questionToken
                        ? factory.createToken(SyntaxKind.QuestionToken)
                        : undefined,
                    param.type ? cloneNode(param.type) : factory.createKeywordTypeNode(SyntaxKind.AnyKeyword),
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
            returnTypeNode = factory.createTypeLiteralNode([
                factory.createPropertySignature(undefined, factory.createIdentifier('value'), undefined, typeNode),
            ])
        }
        parsedLogic.actions.push({ name, parameters, returnTypeNode })
    }
}
