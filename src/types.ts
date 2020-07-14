import * as ts from 'typescript'

export interface ActionTransform {
    name: string
    parameters: ts.ParameterDeclaration[]
    returnTypeNode: ts.TypeNode
}

export interface ReducerTransform {
    name: string
    typeNode: ts.TypeNode | ts.KeywordTypeNode | ts.ParenthesizedTypeNode
}

export interface SelectorTransform {
    name: string
    typeNode: ts.TypeNode | ts.KeywordTypeNode | ts.ParenthesizedTypeNode
    functionTypes: { name: string, type: ts.TypeNode }[]
}

export interface ParsedLogic {
    fileName: string
    logicName: string
    logicTypeArguments: string[]
    checker: ts.TypeChecker
    actions: ActionTransform[]
    reducers: ReducerTransform[]
    selectors: SelectorTransform[]
}
