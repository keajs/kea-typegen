import { ParsedLogic } from '../types'
import * as ts from 'typescript'

export function visitActions(type: ts.Type, parsedLogic: ParsedLogic) {
    const { checker } = parsedLogic
    const properties = checker.getPropertiesOfType(type)

    for (const property of properties) {
        const name = property.getName()
        const type = checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration!)
        const typeNode = checker.typeToTypeNode(type)
        const signature = type.getCallSignatures()[0]

        parsedLogic.actions.push({ name, type, typeNode, signature })
    }
}
