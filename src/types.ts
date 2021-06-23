import * as ts from 'typescript'

export interface ActionTransform {
    name: string
    parameters: ts.ParameterDeclaration[]
    returnTypeNode: ts.TypeNode
}

export interface NameType {
    name: string
    typeNode: ts.TypeNode | ts.KeywordTypeNode | ts.ParenthesizedTypeNode
}

export interface ReducerTransform extends NameType {
    reducerOptions?: ts.TypeNode | ts.ParenthesizedTypeNode
}

export interface SelectorTransform extends NameType {
    functionTypes?: { name: string; type: ts.TypeNode }[]
}

export interface ListenerTransform {
    name: string
    action: ts.TypeNode | ts.KeywordTypeNode | ts.ParenthesizedTypeNode
    payload: ts.TypeNode | ts.KeywordTypeNode | ts.ParenthesizedTypeNode
}

export interface ParsedLogic {
    node: ts.Node,
    fileName: string
    typeFileName: string
    logicName: string
    logicTypeName: string
    logicTypeImported: boolean
    path: string[]
    pathString: string
    logicTypeArguments: string[]
    constants: string[]
    events: Record<string, boolean>
    checker: ts.TypeChecker
    actions: ActionTransform[]
    reducers: ReducerTransform[]
    selectors: SelectorTransform[]
    listeners: ListenerTransform[]
    sharedListeners: ListenerTransform[]
    extraActions: Record<string, ts.TypeNode>
    propsType?: ts.TypeNode
    keyType?: ts.TypeNode
    typeReferencesToImportFromFiles: Record<string, Set<string>>
    typeReferencesInLogicInput: Set<string>
    interfaceDeclaration?: ts.InterfaceDeclaration
}

export interface AppOptions {
    tsConfigPath?: string
    sourceFilePath?: string
    rootPath?: string
    typesPath?: string
    write?: boolean
    watch?: boolean
    quiet?: boolean
    verbose?: boolean
    noImport?: boolean

    log: (message: string) => void
}
