import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'

export function visitActions(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic
    const properties = checker.getPropertiesOfType(type)

    for (const property of properties) {
        const name = property.getName()
        const type = checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration!)
        const typeNode = checker.typeToTypeNode(type, undefined, undefined)
        const signature = type.getCallSignatures()[0]

        let returnTypeNode
        let parameters

        if (ts.isFunctionTypeNode(typeNode)) {
            parameters = signature.getDeclaration().parameters.map((param) => {
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
            returnTypeNode = cloneNode(checker.typeToTypeNode(signature.getReturnType(), undefined, undefined))
        } else {
            returnTypeNode = ts.createTypeLiteralNode([
                ts.createPropertySignature(undefined, ts.createIdentifier('value'), undefined, typeNode, undefined),
            ])
        }

        parsedLogic.actions.push({ name, parameters, returnTypeNode })
    }
}
