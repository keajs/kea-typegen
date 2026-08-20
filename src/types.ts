import * as ts from 'typescript'
import { CloneNodeOptions } from 'ts-clone-node'

export interface ActionTransform {
    name: string
    parameters: ts.ParameterDeclaration[]
    returnTypeNode: ts.TypeNode
    /** Name of the logic this action was connected from, e.g. "userLogic" */
    sourceLogic?: string
}

export interface NameType {
    name: string
    typeNode: ts.TypeNode | ts.KeywordTypeNode | ts.ParenthesizedTypeNode
    /** Name of the logic this value was connected from, e.g. "userLogic" */
    sourceLogic?: string
}

export interface ReducerTransform extends NameType {}

export interface SelectorTransform extends NameType {
    functionTypes?: { name: string; type: ts.TypeNode }[]
}

/** A selector combiner parameter without a type annotation, plus the inferred type to write into the source */
export interface SelectorParamAnnotation {
    parameter: ts.ParameterDeclaration
    typeNode: ts.TypeNode
    /** the value this parameter comes from when the selector input is `s.<name>`, e.g. `counter` */
    sourceValue?: string
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
    selectorParamAnnotations: SelectorParamAnnotation[]
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
    /** Delete logicType.ts files without a logic.ts */
    delete?: boolean
    /** Add @ts-nocheck inside logicType.ts files */
    addTsNocheck?: boolean
    /** Convert kea 2.0 logic input to kea 3.0 builders */
    convertToBuilders?: boolean
    /** Write logic types as MakeLogicType blocks above each kea() call, instead of into logicType.ts files */
    inline?: boolean
    /** Like `inline`, but only for logic files under these paths. Everything else keeps its logicType.ts file. */
    inlinePaths?: string[]
    /** Show TypeScript errors */
    showTsErrors?: boolean
    /** Cache generated logic files into .typegen, use them if generating a logic type for the first time */
    useCache?: boolean
    /** Skip Prettier formatting while generating logic type files */
    prettier?: boolean

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
