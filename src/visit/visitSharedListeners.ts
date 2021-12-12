import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { cloneNode } from '@wessberg/ts-clone-node'
import { gatherImports } from '../utils'

export function visitSharedListeners(type: ts.Type, inputProperty: ts.PropertyAssignment, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        const value = (property.valueDeclaration as ts.PropertyAssignment).initializer

        if (value && ts.isFunctionLike(value)) {
            let typeNode
            const firstParameter = value.parameters[0]
            if (firstParameter) {
                typeNode = cloneNode(
                    checker.typeToTypeNode(checker.getTypeAtLocation(firstParameter), undefined, undefined),
                )
                gatherImports(typeNode, checker, parsedLogic)
            }

            parsedLogic.sharedListeners.push({
                name,
                payload: typeNode,
                action: ts.factory.createTypeLiteralNode([
                    ts.factory.createPropertySignature(
                        undefined,
                        ts.factory.createIdentifier('type'),
                        undefined,
                        ts.factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                    ),
                    ts.factory.createPropertySignature(
                        undefined,
                        ts.factory.createIdentifier('payload'),
                        undefined,
                        typeNode || ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        // ts.factory.createKeywordTypeNode(SyntaxKind.AnyKeyword),
                    ),
                ]),
            })
        }
    }
}
