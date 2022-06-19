import * as ts from 'typescript'
import { CloneNodeOptions } from '@wessberg/ts-clone-node'

export interface ActionTransform {
    name: string
    parameters: ts.ParameterDeclaration[]
    returnTypeNode: ts.TypeNode
}

export interface NameType {
    name: string
    typeNode: ts.TypeNode | ts.KeywordTypeNode | ts.ParenthesizedTypeNode
}

export interface ReducerTransform extends NameType {}

export interface SelectorTransform extends NameType {
    functionTypes?: { name: string; type: ts.TypeNode }[]
}

export interface ListenerTransform {
    name: string
    action: ts.TypeNode | ts.KeywordTypeNode | ts.ParenthesizedTypeNode
    payload: ts.TypeNode | ts.KeywordTypeNode | ts.ParenthesizedTypeNode
}

export interface ParsedLogic {
    node: ts.Node
    fileName: string
    typeFileName: string
    logicName: string
    logicTypeName: string
    logicTypeImported: boolean
    path: string[]
    pathString: string
    hasPathInLogic: boolean
    hasKeyInLogic: boolean
    logicTypeArguments: string[]
    events: Record<string, boolean>
    checker: ts.TypeChecker
    actions: ActionTransform[]
    reducers: ReducerTransform[]
    selectors: SelectorTransform[]
    listeners: ListenerTransform[]
    sharedListeners: ListenerTransform[]
    propsType?: ts.TypeNode
    keyType?: ts.TypeNode
    typeReferencesToImportFromFiles: Record<string, Set<string>>
    interfaceDeclaration?: ts.InterfaceDeclaration
    extraActions: Record<string, ts.TypeNode>
    extraInput: Record<string, { typeNode: ts.TypeNode; withLogicFunction: boolean }>
    extraLogicFields: Record<string, ts.TypeNode>
    importFromKeaInLogicType: Set<string>
    inputBuilderArray: boolean
}

export interface AppOptions {
    tsConfigPath?: string
    packageJsonPath?: string
    sourceFilePath?: string
    rootPath?: string
    typesPath?: string
    write?: boolean
    watch?: boolean
    quiet?: boolean
    verbose?: boolean
    /** Do not write imports inside logic.ts files */
    noImport?: boolean
    /** Add import statements in logicType.ts files for global types (e.g. @types/node) */
    importGlobalTypes?: boolean
    /** List of paths we will never import from inside logicType.ts files */
    ignoreImportPaths?: string[]
    /** Write paths inside logic.ts files */
    writePaths?: boolean
    /** Add @ts-nocheck inside logicType.ts files */
    addTsNocheck?: boolean
    /** Convert kea 2.0 logic input to kea 3.0 builders */
    convertToBuilders?: boolean

    log: (message: string) => void
}

export interface VisitKeaPropertyArguments {
    name: string
    node: ts.Node
    type: ts.Type
    typeNode: ts.TypeNode
    parsedLogic: ParsedLogic
    appOptions: AppOptions
    checker: ts.TypeChecker
    gatherImports(input: ts.Node): void
    cloneNode(node: ts.Node | undefined, options?: Partial<CloneNodeOptions<ts.Node>>): ts.Node | undefined
    getTypeNodeForNode(node: ts.Node): ts.TypeNode
    prepareForPrint<T extends ts.Node>(node: T): T
}

export type TypeBuilder = (args: VisitKeaPropertyArguments) => void
export interface TypeBuilderModule {
    name: string
    file: string
    typeBuilder?: TypeBuilder
}

export interface Plugin {
    visitKeaProperty?: (args: VisitKeaPropertyArguments) => void
}
export interface PluginModule extends Plugin {
    name: string
    file: string
    typeBuilder?: (args: VisitKeaPropertyArguments) => void
}
