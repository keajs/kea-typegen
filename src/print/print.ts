import {
    createPrinter,
    createSourceFile,
    EmitHint,
    factory,
    Node,
    NewLineKind,
    Program,
    ScriptKind,
    ScriptTarget,
    SyntaxKind,
    TypeNode,
} from 'typescript'
import * as fs from 'fs'
import * as path from 'path'
import * as prettier from 'prettier'

import { AppOptions, ParsedLogic } from '../types'
import { printActions } from './printActions'
import { printAsyncActions } from './printAsyncActions'
import { printReducers } from './printReducers'
import { printReducer } from './printReducer'
import { printSelector } from './printSelector'
import { printSelectors } from './printSelectors'
import { printValues } from './printValues'
import { printInternalSelectorTypes } from './printInternalSelectorTypes'
import { printActionKeys } from './printActionKeys'
import { printActionTypes } from './printActionTypes'
import { printInternalReducerActions } from './printInternalReducerActions'
import { printActionCreators } from './printActionCreators'
import { printProps } from './printProps'
import { printKey } from './printKey'
import { printDefaults } from './printDefaults'
import { printEvents } from './printEvents'
import { printSharedListeners } from './printSharedListeners'
import { printListeners } from './printListeners'
import { writePaths } from '../write/writePaths'
import { writeTypeImports } from '../write/writeTypeImports'
import { printInternalExtraInput } from './printInternalExtraInput'
import { convertToBuilders } from '../write/convertToBuilders'

export function runThroughPrettier(sourceText: string, filePath: string): string {
    const options = prettier.resolveConfig.sync(filePath)
    if (options) {
        try {
            return prettier.format(sourceText, { ...options, filepath: filePath })
        } catch (e) {
            console.error(`!! Prettier: Error formatting "${filePath}"`)
            console.error(e.message)
            return sourceText
        }
    } else {
        return sourceText
    }
}

// returns files to write
export function printToFiles(
    program: Program,
    appOptions: AppOptions,
    parsedLogics: ParsedLogic[],
): { filesToWrite: number; writtenFiles: number; filesToModify: number } {
    const { log } = appOptions

    const groupedByFile: Record<string, ParsedLogic[]> = {}
    for (const parsedLogic of parsedLogics) {
        if (!groupedByFile[parsedLogic.fileName]) {
            groupedByFile[parsedLogic.fileName] = []
        }
        groupedByFile[parsedLogic.fileName].push(parsedLogic)

        // create the Nodes and gather referenced types
        printLogicType(parsedLogic, appOptions)
    }

    // Automatically ignore imports from "node_modules/@types/node", if {types: ["node"]} in tsconfig.json
    const defaultGlobalTypePaths = appOptions.importGlobalTypes
        ? []
        : (program.getCompilerOptions().types || []).map(
              (type) =>
                  path.join(
                      appOptions.packageJsonPath ? path.dirname(appOptions.packageJsonPath) : appOptions.rootPath,
                      'node_modules',
                      '@types',
                      type,
                  ) + path.sep,
          )

    // Manually ignored
    const ignoredImportPaths = (appOptions.ignoreImportPaths || []).map((importPath) =>
        path.resolve(appOptions.rootPath, importPath),
    )

    const doNotImportFromPaths = [...defaultGlobalTypePaths, ...ignoredImportPaths]

    const shouldIgnore = (absolutePath: string) =>
        !!doNotImportFromPaths.find((badPath) => absolutePath.startsWith(badPath))

    let writtenFiles = 0
    let filesToWrite = 0
    let filesToModify = 0

    Object.entries(groupedByFile).forEach(([fileName, parsedLogics]) => {
        const typeFileName = parsedLogics[0].typeFileName

        const logicStrings = []
        const requiredKeys = new Set(['Logic'])
        for (const parsedLogic of parsedLogics) {
            const logicTypeStirng = runThroughPrettier(nodeToString(parsedLogic.interfaceDeclaration), typeFileName)
            logicStrings.push(logicTypeStirng)
            for (const string of parsedLogic.importFromKeaInLogicType.values()) {
                requiredKeys.add(string)
            }
            if (parsedLogic.sharedListeners.length > 0) {
                requiredKeys.add('BreakPointFunction')
            }
        }

        const output = logicStrings.join('\n\n')

        const nodeModulesPath = path.join(
            appOptions.packageJsonPath ? path.dirname(appOptions.packageJsonPath) : appOptions.rootPath,
            'node_modules',
        )

        const otherimports = Object.entries(parsedLogics[0].typeReferencesToImportFromFiles)
            .filter(([_, list]) => list.size > 0)
            .map(([file, list]) => {
                let finalPath = file

                // Relative path? Get the absolute.
                if (finalPath.startsWith('.')) {
                    finalPath = path.resolve(path.dirname(parsedLogics[0].typeFileName), finalPath)
                }
                if (finalPath.startsWith('node_modules/')) {
                    finalPath = path.resolve(path.dirname(nodeModulesPath), finalPath)
                }
                // clean up '../../node_modules/...'
                if (finalPath.startsWith(nodeModulesPath)) {
                    finalPath = finalPath.substring(nodeModulesPath.length + 1)
                    if (finalPath.startsWith('.pnpm/')) {
                        // node_modules/.pnpm/pkg@version/node_modules/* --> *
                        const regex = /\.pnpm\/[^/]+@[^\/]+\/node_modules\/(.*)/
                        const result = finalPath.match(regex)
                        if (result && result.length > 1) {
                            finalPath = result[1]
                        }
                    }
                    if (finalPath.startsWith('@types/')) {
                        finalPath = finalPath.substring(7)
                    }
                }

                // Resolve absolute urls
                if (finalPath.startsWith('/')) {
                    finalPath = path.relative(path.dirname(parsedLogics[0].typeFileName), finalPath)
                    if (!finalPath.startsWith('.')) {
                        finalPath = `./${finalPath}`
                    }
                }

                // Remove extension
                finalPath = finalPath.replace(/(\.d|)\.tsx?$/, '')

                // Remove "/index"
                if (finalPath.split('/').length === 2 && finalPath.endsWith('/index')) {
                    finalPath = finalPath.substring(0, finalPath.length - 6)
                }

                return {
                    list: [...list].sort(),
                    fullPath: file,
                    finalPath,
                }
            })
            .filter((entry) => !shouldIgnore(entry.fullPath))
            .map(({ list, finalPath }) => `import type { ${list.join(', ')} } from '${finalPath}'`)
            .join('\n')

        const finalOutput = [
            [
                `// Generated by kea-typegen on ${new Date().toUTCString()}. DO NOT EDIT THIS FILE MANUALLY.`,
                appOptions.addTsNocheck ? '// @ts-nocheck' : undefined,
            ]
                .filter((a) => !!a)
                .join('\n'),
            `import type { ${[...requiredKeys.values()].join(', ')} } from 'kea'`,
            otherimports,
            output,
        ]
            .filter((a) => !!a)
            .join('\n\n')

        // write the logic type

        let existingOutput

        try {
            existingOutput = fs.readFileSync(typeFileName)?.toString()
        } catch (error) {}

        if (
            !existingOutput ||
            (existingOutput &&
                existingOutput.split('\n').slice(1).join('\n') !== finalOutput.split('\n').slice(1).join('\n'))
        ) {
            filesToWrite += 1
            if (appOptions.write) {
                fs.mkdirSync(path.dirname(typeFileName), { recursive: true })
                fs.writeFileSync(typeFileName, finalOutput)
                writtenFiles += 1
                log(`🔥 Writing: ${path.relative(process.cwd(), typeFileName)}`)
            } else {
                log(`❌ Will not write: ${path.relative(process.cwd(), typeFileName)}`)
            }
        } else {
            if (appOptions.verbose) {
                log(`🤷 Unchanged: ${path.relative(process.cwd(), typeFileName)}`)
            }
        }

        const parsedLogicNeedsTypeImport = (pl: ParsedLogic) =>
            // reload if logic type not imported
            (pl.logicTypeImported === false ||
                // reload if don't have the right types in arguments
                pl.logicTypeArguments.length > 0) &&
            pl.fileName.match(/\.tsx?$/)

        // write the type into the logic itself
        const logicsNeedingImports = parsedLogics.filter(parsedLogicNeedsTypeImport)
        if (logicsNeedingImports.length > 0) {
            if (appOptions.write && !appOptions.noImport) {
                writeTypeImports(appOptions, program, fileName, logicsNeedingImports, parsedLogics)
                filesToModify += logicsNeedingImports.length
            } else {
                log(
                    `❌ Will not write ${logicsNeedingImports.length} logic type import${
                        logicsNeedingImports.length === 1 ? '' : 's'
                    }`,
                )
            }
        }

        // add a path if needed
        const parsedLogicNeedsPath = appOptions.writePaths ? (pl: ParsedLogic) => !pl.hasPathInLogic : () => false
        const logicsNeedingPaths = parsedLogics.filter(parsedLogicNeedsPath)
        if (logicsNeedingPaths.length > 0) {
            if (appOptions.write && !appOptions.noImport) {
                writePaths(appOptions, program, fileName, logicsNeedingPaths)
                filesToModify += logicsNeedingPaths.length
            } else {
                log(
                    `❌ Will not write ${logicsNeedingPaths.length} logic path${
                        logicsNeedingPaths.length === 1 ? '' : 's'
                    }`,
                )
            }
        }

        // convert to logic builder
        const parsedLogicNeedsConversion = appOptions.convertToBuilders
            ? (pl: ParsedLogic) => !pl.inputBuilderArray
            : () => false
        const logicsNeedingConversion = parsedLogics.filter(parsedLogicNeedsConversion)
        if (logicsNeedingConversion.length > 0) {
            if (appOptions.write && !appOptions.noImport) {
                convertToBuilders(appOptions, program, fileName, logicsNeedingConversion)
                filesToModify += logicsNeedingConversion.length
            } else {
                log(
                    `❌ Will not write ${logicsNeedingConversion.length} logic path${
                        logicsNeedingConversion.length === 1 ? '' : 's'
                    }`,
                )
            }
        }
    })

    if (writtenFiles === 0 && filesToModify === 0) {
        if (appOptions.write) {
            log(`💚 ${parsedLogics.length} logic type${parsedLogics.length === 1 ? '' : 's'} up to date!`)
            log('')
        } else if (filesToWrite > 0 || filesToModify > 0) {
            log(
                `🚨 Run "kea-typegen write" to save ${filesToWrite + filesToModify} file${
                    filesToWrite === 1 ? '' : 's'
                } to disk`,
            )
        }
    }

    return { filesToWrite, writtenFiles, filesToModify }
}

export function nodeToString(node: Node): string {
    const printer = createPrinter({ newLine: NewLineKind.LineFeed })
    const sourceFile = createSourceFile('logic.ts', '', ScriptTarget.Latest, false, ScriptKind.TS)
    return printer.printNode(EmitHint.Unspecified, node, sourceFile)
}

export function parsedLogicToTypeString(parsedLogic: ParsedLogic, appOptions?: AppOptions): string {
    printLogicType(parsedLogic, appOptions)
    return nodeToString(parsedLogic.interfaceDeclaration)
}

export function printLogicType(parsedLogic: ParsedLogic, appOptions?: AppOptions): void {
    const addSelectorTypeHelp = parsedLogic.selectors.filter((s) => s.functionTypes.length > 0).length > 0

    const logicProperties: Record<string, TypeNode | null> = {
        actionCreators: printActionCreators(parsedLogic, appOptions),
        actionKeys: printActionKeys(parsedLogic, appOptions),
        actionTypes: printActionTypes(parsedLogic, appOptions),
        actions: printActions(parsedLogic, appOptions),
        asyncActions: printAsyncActions(parsedLogic, appOptions),
        defaults: printDefaults(parsedLogic),
        events: printEvents(parsedLogic),
        key: printKey(parsedLogic),
        listeners: printListeners(parsedLogic),
        path: factory.createTupleTypeNode(
            parsedLogic.path.map((p) => factory.createLiteralTypeNode(factory.createStringLiteral(p))),
        ),
        pathString: factory.createLiteralTypeNode(factory.createStringLiteral(parsedLogic.pathString)),
        props: printProps(parsedLogic),
        reducer: printReducer(parsedLogic),
        reducers: printReducers(parsedLogic),
        selector: printSelector(parsedLogic),
        selectors: printSelectors(parsedLogic),
        sharedListeners: printSharedListeners(parsedLogic),
        values: printValues(parsedLogic),
    }
    for (const [name, typeNode] of Object.entries(parsedLogic.extraLogicFields)) {
        if (name in logicProperties) {
            console.error(`❗ Can not add extra logic field ${name} because this field is already in the logic.`)
        } else {
            logicProperties[name] = typeNode
        }
    }
    const logicMetaProperties: Record<string, TypeNode | null> = {
        _isKea: factory.createLiteralTypeNode(factory.createTrue()),
        _isKeaWithKey: factory.createLiteralTypeNode(
            parsedLogic.keyType ? factory.createTrue() : factory.createFalse(),
        ),
        __keaTypeGenInternalSelectorTypes: addSelectorTypeHelp ? printInternalSelectorTypes(parsedLogic) : null,
        __keaTypeGenInternalReducerActions:
            Object.keys(parsedLogic.extraActions).length > 0 ? printInternalReducerActions(parsedLogic) : null,
        __keaTypeGenInternalExtraInput:
            Object.keys(parsedLogic.extraInput).length > 0 ? printInternalExtraInput(parsedLogic) : null,
    }

    const sortedLogicProperties = {
        ...Object.fromEntries(
            Object.entries(logicProperties).sort((a, b) => (a[0] === b[0] ? 0 : a[0] < b[0] ? -1 : 1)),
        ),
        ...logicMetaProperties,
    }

    parsedLogic.interfaceDeclaration = factory.createInterfaceDeclaration(
        [factory.createModifier(SyntaxKind.ExportKeyword)],
        factory.createIdentifier(`${parsedLogic.logicName}Type`),
        undefined,
        [
            factory.createHeritageClause(SyntaxKind.ExtendsKeyword, [
                factory.createExpressionWithTypeArguments(factory.createIdentifier('Logic'), undefined),
            ]),
        ],
        Object.entries(sortedLogicProperties)
            .filter(([_, value]) => !!value)
            .map(([name, typeNode]) =>
                factory.createPropertySignature(undefined, factory.createIdentifier(name), undefined, typeNode),
            ),
    )
}
