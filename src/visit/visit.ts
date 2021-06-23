import * as ts from 'typescript'
import * as path from 'path'
import { AppOptions, ParsedLogic } from '../types'
import { gatherImports, getFilenameForImportSpecifier, getLogicPathString, isKeaCall } from '../utils'
import { visitActions } from './visitActions'
import { visitReducers } from './visitReducers'
import { visitSelectors } from './visitSelectors'
import { visitLoaders } from './visitLoaders'
import { visitConnect } from './visitConnect'
import { visitWindowValues } from './visitWindowValues'
import { visitProps } from './visitProps'
import { visitKey } from './visitKey'
import { visitPath } from './visitPath'
import { visitListeners } from './visitListeners'
import { visitConstants } from './visitConstants'
import { visitEvents } from './visitEvents'
import { visitDefaults } from './visitDefaults'
import { visitSharedListeners } from './visitSharedListeners'

const visitFunctions = {
    actions: visitActions,
    connect: visitConnect,
    constants: visitConstants,
    defaults: visitDefaults,
    events: visitEvents,
    key: visitKey,
    listeners: visitListeners,
    loaders: visitLoaders,
    path: visitPath,
    props: visitProps,
    reducers: visitReducers,
    selectors: visitSelectors,
    sharedListeners: visitSharedListeners,
    windowValues: visitWindowValues,
}

export function visitProgram(program: ts.Program, appOptions?: AppOptions): ParsedLogic[] {
    const checker = program.getTypeChecker()
    const parsedLogics: ParsedLogic[] = []

    for (const sourceFile of program.getSourceFiles()) {
        if (!sourceFile.isDeclarationFile && !sourceFile.fileName.endsWith('Type.ts')) {
            if (appOptions?.verbose) {
                appOptions.log(`👀 Visiting: ${path.relative(process.cwd(), sourceFile.fileName)}`)
            }
            ts.forEachChild(sourceFile, createVisit(checker, parsedLogics, sourceFile, appOptions))
        }
    }

    return parsedLogics
}

export function createVisit(
    checker: ts.TypeChecker,
    parsedLogics: ParsedLogic[],
    sourceFile: ts.SourceFile,
    appOptions?: AppOptions,
) {
    return function visit(node: ts.Node) {
        if (!isKeaCall(node, checker)) {
            ts.forEachChild(node, visit)
            return
        }

        let logicName = 'logic'
        if (ts.isCallExpression(node.parent) && ts.isVariableDeclaration(node.parent.parent)) {
            logicName = node.parent.parent.name.getText()
        }

        const logicTypeName = `${logicName}Type`

        let logicTypeArguments = []
        let logicTypeImported = false

        // get "logicType" in "kea<logicType>(..)"
        const keaTypeArguments = ts.isCallExpression(node.parent) ? node.parent.typeArguments : []
        const keaTypeArgument = keaTypeArguments?.[0]

        const pathString = getLogicPathString(appOptions, sourceFile.fileName)
        let typeFileName = sourceFile.fileName.replace(/\.[tj]sx?$/, 'Type.ts')

        if (keaTypeArgument?.typeName?.escapedText === logicTypeName) {
            // kea<logicType<somethingElse>>(...)
            // store <somethingElse> on the generated type!
            if (keaTypeArgument.typeArguments && keaTypeArgument.typeArguments.length > 0) {
                logicTypeArguments = (keaTypeArgument.typeArguments as ts.Node[]).map((a) => a.getFullText().trim())
            }

            // only if symbol resolves we mark the logic type as imported
            const symbol = checker.getSymbolAtLocation(keaTypeArgument.typeName)
            if (symbol) {
                const declaration = symbol.getDeclarations()[0]

                if (ts.isImportSpecifier(declaration)) {
                    const filename = getFilenameForImportSpecifier(declaration, checker)
                    logicTypeImported = filename === typeFileName
                }
            }
        }

        if (appOptions?.rootPath && appOptions?.typesPath) {
            const relativePathFromRoot = path.relative(appOptions.rootPath, typeFileName)
            typeFileName = path.resolve(appOptions.typesPath, relativePathFromRoot)
        }

        const parsedLogic: ParsedLogic = {
            checker,
            node,
            logicName,
            logicTypeImported,
            logicTypeName,
            logicTypeArguments,
            fileName: sourceFile.fileName,
            typeFileName,
            actions: [],
            reducers: [],
            selectors: [],
            constants: [],
            listeners: [],
            sharedListeners: [],
            events: {},
            extraActions: {},
            keyType: undefined,
            propsType: undefined,
            path: pathString.split('.'),
            pathString: pathString,
            typeReferencesToImportFromFiles: {},
            typeReferencesInLogicInput: new Set(),
            // typeReferencesInCreatedLogicType: new Set(),
        }

        const input = (node.parent as ts.CallExpression).arguments[0] as ts.ObjectLiteralExpression

        for (const inputProperty of input.properties) {
            const symbol = checker.getSymbolAtLocation(inputProperty.name as ts.Identifier)

            if (!symbol) {
                continue
            }

            const name = symbol.getName()
            let type = checker.getTypeOfSymbolAtLocation(symbol, symbol.valueDeclaration!)
            let typeNode = type ? checker.typeToTypeNode(type, undefined, undefined) : null

            if (typeNode && ts.isFunctionTypeNode(typeNode)) {
                type = type.getCallSignatures()[0].getReturnType()
            }

            visitFunctions[name]?.(type, inputProperty, parsedLogic)
        }

        parsedLogics.push(parsedLogic)
    }
}
