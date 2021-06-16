import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import * as path from 'path'
import { NodeBuilderFlags } from 'typescript'
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
                if (parsedLogic.logicName === 'githubLogic' && name === 'setRepositories') {
                    // debugger
                    const type = checker.getTypeAtLocation((param.type as ts.ArrayTypeNode).elementType)
                    const symbol = type.getSymbol()
                    const files = symbol.declarations.map(d => d.getSourceFile().fileName).filter(fileName => !fileName.includes('/node_modules/'))
                    const declaration = symbol.getDeclarations()[0]

                    const isExported = declaration.parent.modifiers[0]?.kind === 92

                    if (isExported && files.length === 1 && files[0] !== parsedLogic.fileName) {
                        if (!parsedLogic.typeImports[files[0]]) {
                            parsedLogic.typeImports[files[0]] = new Set()
                        }
                        parsedLogic.typeImports[files[0]].add(type.aliasSymbol.escapedName as string)
                    }
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

            const sigReturnType = signature.getReturnType()
            const sigReturnTypeNode = checker.typeToTypeNode(sigReturnType, undefined, NodeBuilderFlags.NoTruncation)
            returnTypeNode = cloneNode(sigReturnTypeNode)
        } else {
            returnTypeNode = ts.createTypeLiteralNode([
                ts.createPropertySignature(undefined, ts.createIdentifier('value'), undefined, typeNode, undefined),
            ])
        }

        parsedLogic.actions.push({ name, parameters, returnTypeNode })
    }
}
