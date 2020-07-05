import * as ts from 'typescript'
import { ParsedLogic } from './types'

export function parseActionProperty(property: ts.Symbol, checker: ts.TypeChecker, parsedLogic: ParsedLogic) {
    const name = property.getName()
    const type = checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration!)
    const typeNode = checker.typeToTypeNode(type) as ts.TypeNode
    const signature = type.getCallSignatures()[0]

    parsedLogic.actions.push({ name, type, typeNode, signature })

    // console.log(checker.typeToString(type))
    // const returnType = signature.getReturnType()
    // console.log(checker.typeToString(returnType))
}

export function parseReducerProperty(property: ts.Symbol, checker: ts.TypeChecker, parsedLogic: ParsedLogic) {
    const name = property.getName()
    const type = checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration!)
    console.log(checker.typeToString(type))
    const typeNode = checker.typeToTypeNode(type) as ts.TypeNode

    // typeNode.elementType.type.types[0]

    // if (ts.IntersectionTypeNode(typeNode)){
    //     //typeNode.
    //     // const typeOfFirstInArray = (type as ts.TypeReference).resolvedTypeArguments?.[0]?.types?.[0]
    //
    //     // reducers.push({name, type: typeOfFirstInArray})
    //     console.log('.')
    // }

    console.log('.')
}
