import { ParsedLogic } from '../types'
import * as ts from 'typescript'
import { gatherImports } from '../utils'
import {Expression, NodeBuilderFlags, Type} from 'typescript'

export function visitWindowValues(parsedLogic: ParsedLogic, type: Type, expression: Expression) {
    const { checker } = parsedLogic

    for (const property of type.getProperties()) {
        const name = property.getName()
        if (ts.isPropertyAssignment(property.valueDeclaration)) {
            const value = property.valueDeclaration.initializer

            if (value && ts.isFunctionLike(value)) {
                const type = checker.getTypeOfSymbolAtLocation(property, value)
                const signature = type.getCallSignatures()[0]
                const typeNode = checker.typeToTypeNode(signature.getReturnType(), undefined, NodeBuilderFlags.NoTruncation)
                gatherImports(typeNode, checker, parsedLogic)

                parsedLogic.reducers.push({ name, typeNode })
            }
        }
    }
}
