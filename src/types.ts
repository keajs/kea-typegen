import * as ts from 'typescript'

export interface ActionTransform {
    name: string
    type: ts.Type
    signature: ts.Signature
    typeNode: ts.TypeNode
}

export interface ReducerTransform {
    name: string
    type: ts.Type | undefined
}

export interface ParsedLogic {
    fileName: string
    logicName: string
    checker: ts.TypeChecker
    actions: ActionTransform[]
    reducers: ReducerTransform[]
}
