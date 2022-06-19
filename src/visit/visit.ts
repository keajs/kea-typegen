import * as ts from 'typescript'
import * as path from 'path'
import { AppOptions, ParsedLogic, PluginModule, TypeBuilderModule, VisitKeaPropertyArguments } from '../types'
import {
    gatherImports,
    getFilenameForImportDeclaration,
    getFilenameForNode,
    getLogicPathString,
    getTypeNodeForNode,
    isKeaCall,
} from '../utils'
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
import { visitEvents } from './visitEvents'
import { visitDefaults } from './visitDefaults'
import { visitSharedListeners } from './visitSharedListeners'
import { cloneNode } from 'ts-clone-node'

const visitFunctions = {
    actions: visitActions,
    connect: visitConnect,
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
    const pluginModules: PluginModule[] = []
    const typeBuilderModules: TypeBuilderModule[] = []

    for (const sourceFile of program.getSourceFiles()) {
        if (!sourceFile.isDeclarationFile && !sourceFile.fileName.endsWith('Type.ts')) {
            ts.forEachChild(sourceFile, visitResetContext(checker, pluginModules))
        }
    }

    for (const sourceFile of program.getSourceFiles()) {
        if (!sourceFile.isDeclarationFile && !sourceFile.fileName.endsWith('Type.ts')) {
            if (appOptions?.verbose) {
                appOptions.log(`👀 Visiting: ${path.relative(process.cwd(), sourceFile.fileName)}`)
            }
            ts.forEachChild(
                sourceFile,
                visitKeaCalls(checker, parsedLogics, sourceFile, appOptions, pluginModules, typeBuilderModules),
            )
        }
    }

    return parsedLogics
}

export function visitResetContext(checker: ts.TypeChecker, pluginModules: PluginModule[]) {
    return function visit(node: ts.Node) {
        // find a `resetContext` call
        if (
            !ts.isIdentifier(node) ||
            !node.parent ||
            !ts.isCallExpression(node.parent) ||
            node.getText() !== 'resetContext'
        ) {
            ts.forEachChild(node, visit)
            return
        }

        // find the `plugins` property of resetContext
        const callArgument = node.parent.arguments?.[0]
        if (ts.isObjectLiteralExpression(callArgument)) {
            for (const prop of callArgument.properties) {
                if (ts.isPropertyAssignment(prop) && prop.name.getText() === 'plugins') {
                    if (ts.isArrayLiteralExpression(prop.initializer)) {
                        // gather typegen modules for plugins that have any
                        for (const plugin of prop.initializer.elements) {
                            const identifier = ts.isCallExpression(plugin) ? plugin.expression : plugin
                            const pluginName = identifier.getText()
                            const symbol = checker.getSymbolAtLocation(identifier)

                            if (symbol && !pluginModules.find(({ name }) => name === pluginName)) {
                                for (let declaration of symbol.getDeclarations()) {
                                    let decNode: ts.Node = declaration
                                    while (decNode) {
                                        // find if it's an imported plugin
                                        if (ts.isImportDeclaration(decNode)) {
                                            const filename = getFilenameForImportDeclaration(checker, decNode)
                                            if (!filename) {
                                                break
                                            }
                                            const folder = path.dirname(filename)

                                            for (const filename of ['typegen.js', 'typegen.ts']) {
                                                const typegenFile = path.resolve(folder, filename)

                                                try {
                                                    let typegenModule = require(typegenFile)

                                                    if (typegenModule && typegenModule.default) {
                                                        typegenModule = typegenModule.default
                                                    }

                                                    if (typeof typegenModule?.visitKeaProperty !== 'undefined') {
                                                        pluginModules.push({
                                                            name: pluginName,
                                                            file: typegenFile,
                                                            visitKeaProperty: typegenModule.visitKeaProperty,
                                                        })
                                                        break
                                                    }
                                                } catch (error) {
                                                    if (error.code !== 'MODULE_NOT_FOUND') {
                                                        console.error(error)
                                                    }
                                                }
                                            }
                                        }
                                        decNode = decNode.parent
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

export function visitKeaCalls(
    checker: ts.TypeChecker,
    parsedLogics: ParsedLogic[],
    sourceFile: ts.SourceFile,
    appOptions: AppOptions,
    pluginModules: PluginModule[],
    typeBuilderModules: TypeBuilderModule[],
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
                const declaration = symbol.getDeclarations()?.[0]

                if (declaration && ts.isImportSpecifier(declaration)) {
                    const filename = getFilenameForNode(declaration, checker)
                    logicTypeImported = filename === typeFileName
                }
            }
        }

        if (appOptions?.rootPath && appOptions?.typesPath) {
            const relativePathFromRoot = path.relative(appOptions.rootPath, typeFileName)
            typeFileName = path.resolve(appOptions.typesPath, relativePathFromRoot)
        }

        const input = (node.parent as ts.CallExpression).arguments[0]

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
            listeners: [],
            sharedListeners: [],
            events: {},
            keyType: undefined,
            propsType: undefined,
            path: pathString.split('.'),
            pathString: pathString,
            hasKeyInLogic: false,
            hasPathInLogic: false,
            typeReferencesToImportFromFiles: {},
            extraActions: {},
            extraInput: {},
            extraLogicFields: {},
            importFromKeaInLogicType: new Set([]),
            inputBuilderArray: ts.isArrayLiteralExpression(input),
        }

        const calls: {
            name: string
            type: ts.Type
            typeNode: ts.TypeNode | null
            expression: ts.Expression
            typeBuilders: PluginModule[]
        }[] = []

        if (ts.isObjectLiteralExpression(input)) {
            for (const inputProperty of input.properties) {
                if (!ts.isPropertyAssignment(inputProperty)) {
                    continue
                }
                const symbol = checker.getSymbolAtLocation(inputProperty.name)
                if (!symbol) {
                    continue
                }
                const name = symbol.getName()
                const type = checker.getTypeOfSymbolAtLocation(symbol, symbol.valueDeclaration)
                const typeNode = type ? checker.typeToTypeNode(type, undefined, undefined) : null
                calls.push({ name, type, typeNode, expression: inputProperty.initializer, typeBuilders: [] })
            }
        } else if (ts.isArrayLiteralExpression(input)) {
            for (const callExpression of input.elements) {
                if (!ts.isCallExpression(callExpression) || callExpression.arguments.length === 0) {
                    continue
                }

                const builderName = callExpression.expression.getText()
                const argument = callExpression.arguments[0]
                const type = checker.getTypeAtLocation(argument)
                const typeNode = type ? checker.typeToTypeNode(type, undefined, undefined) : null

                const identifier = callExpression.expression
                const symbol = checker.getSymbolAtLocation(identifier)

                let typeBuilders: TypeBuilderModule[] = []

                if (symbol) {
                    declarations: for (let declaration of symbol.getDeclarations()) {
                        let decNode: ts.Node = declaration
                        while (decNode) {
                            // find if it's an imported builder
                            if (ts.isImportDeclaration(decNode)) {
                                const filename = getFilenameForImportDeclaration(checker, decNode)
                                if (!filename) {
                                    break
                                }
                                let typeBuilderModule = typeBuilderModules.find(
                                    ({ file, name }) => file === filename && name === builderName,
                                )
                                if (typeBuilderModule) {
                                    if (typeBuilderModule.typeBuilder) {
                                        typeBuilders.push(typeBuilderModule)
                                    }
                                    break declarations
                                }

                                const folder = path.dirname(filename)
                                let fileNoExt = path.basename(filename, path.extname(filename))
                                if (fileNoExt.endsWith('.d')) {
                                    fileNoExt = fileNoExt.slice(0, -2)
                                }
                                const pathsToTry = [
                                    'typegen.js',
                                    'typegen.ts',
                                    `${fileNoExt}.typegen.js`,
                                    `${fileNoExt}.typegen.ts`,
                                ].map((filename) => path.resolve(folder, filename))

                                for (const typegenFile of pathsToTry) {
                                    try {
                                        let typeBuilder = require(typegenFile)?.[builderName]
                                        typeBuilderModule = {
                                            name: builderName,
                                            file: filename,
                                            typeBuilder: typeBuilder,
                                        }
                                        typeBuilders.push(typeBuilderModule)
                                        typeBuilderModules.push(typeBuilderModule)
                                        break declarations
                                    } catch (error) {
                                        if (error.code !== 'MODULE_NOT_FOUND') {
                                            console.error(
                                                `[KEA] Error loading type builder "${builderName}" in "${typegenFile}"`,
                                            )
                                            console.error(error)
                                        }
                                    }
                                }
                                typeBuilderModule = {
                                    name: builderName,
                                    file: filename,
                                    typeBuilder: null,
                                }
                                typeBuilderModules.push(typeBuilderModule)
                                break declarations
                            }
                            decNode = decNode.parent
                        }
                    }
                }

                calls.push({
                    name: builderName,
                    type,
                    typeNode,
                    expression: callExpression.arguments[0],
                    typeBuilders: typeBuilders,
                })
            }
        }

        for (let { name, type, typeNode, expression, typeBuilders } of calls) {
            if (name === 'path' || name === 'logicPath') {
                parsedLogic.hasPathInLogic = true
            }
            if (name === 'key') {
                parsedLogic.hasKeyInLogic = true
            }

            if (typeNode && ts.isFunctionTypeNode(typeNode)) {
                type = type.getCallSignatures()[0].getReturnType()
                typeNode = type ? checker.typeToTypeNode(type, undefined, undefined) : null
            }

            visitFunctions[name]?.(parsedLogic, type, expression)

            const visitKeaPropertyArguments: VisitKeaPropertyArguments = {
                name,
                appOptions,
                type,
                typeNode,
                parsedLogic,
                node: expression,
                checker,
                gatherImports: (input) => gatherImports(input, checker, parsedLogic),
                cloneNode,
                getTypeNodeForNode: (node) => getTypeNodeForNode(node, checker),
                prepareForPrint: (node) => {
                    gatherImports(node, checker, parsedLogic)
                    return cloneNode(node)
                },
            }

            for (const pluginModule of typeBuilders) {
                try {
                    pluginModule.typeBuilder?.(visitKeaPropertyArguments)
                } catch (e) {
                    console.error(
                        `!! Problem running "typeBuilder" on plugin "${pluginModule.name}" (${pluginModule.file})`,
                    )
                    console.error(e)
                }
            }

            if (typeBuilders.length === 0) {
                for (const pluginModule of Object.values(pluginModules)) {
                    try {
                        pluginModule.visitKeaProperty?.(visitKeaPropertyArguments)
                    } catch (e) {
                        console.error(
                            `!! Problem running "visitKeaProperty" on plugin "${pluginModule.name}" (${pluginModule.file})`,
                        )
                        console.error(e)
                    }
                }
            }
        }
        parsedLogics.push(parsedLogic)
    }
}
