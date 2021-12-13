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
    TypeParameterDeclaration,
} from 'typescript'
import * as fs from 'fs'
import * as path from 'path'
import * as prettier from 'prettier'

import { AppOptions, ParsedLogic } from '../types'
import { printActions } from './printActions'
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
import { printConstants } from './printConstants'
import { printReducerOptions } from './printReducerOptions'
import { printEvents } from './printEvents'
import { printSharedListeners } from './printSharedListeners'
import { printListeners } from './printListeners'
import { writePaths, writeTypeImports } from '../write/write'
import { printInternalExtraInput } from './printInternalExtraInput'

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
        for (const parsedLogic of parsedLogics) {
            const logicTypeStirng = runThroughPrettier(nodeToString(parsedLogic.interfaceDeclaration), typeFileName)
            logicStrings.push(logicTypeStirng)
        }

        const output = logicStrings.join('\n\n')

        const requiredKeys = ['Logic']
        if (parsedLogics.find((l) => l.sharedListeners.length > 0)) {
            requiredKeys.push('BreakPointFunction')
        }

        const otherimports = Object.entries(parsedLogics[0].typeReferencesToImportFromFiles)
            .filter(([_, list]) => list.size > 0)
            .map(([file, list]) => {
                let relativePath = path.relative(path.dirname(parsedLogics[0].typeFileName), file)
                relativePath = relativePath.replace(/\.tsx?$/, '')
                if (!relativePath.startsWith('.')) {
                    relativePath = `./${relativePath}`
                }
                return {
                    list: [...list].sort(),
                    fullPath: file,
                    relativePath,
                }
            })
            .filter(({ fullPath }) => !shouldIgnore(fullPath))
            .map(({ list, relativePath }) => `import { ${list.join(', ')} } from '${relativePath}'`)
            .join('\n')

        const finalOutput = [
            `// Generated by kea-typegen on ${new Date().toUTCString()}. DO NOT EDIT THIS FILE MANUALLY.`,
            `import { ${requiredKeys.join(', ')} } from 'kea'`,
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
                pl.logicTypeArguments.join(', ') !== [...pl.typeReferencesInLogicInput].sort().join(', ')) &&
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

export function getLogicTypeArguments(parsedLogic: ParsedLogic): TypeParameterDeclaration[] {
    return [...parsedLogic.typeReferencesInLogicInput]
        .sort()
        .map((text) => factory.createTypeParameterDeclaration(factory.createIdentifier(text), undefined))
}

export function printLogicType(parsedLogic: ParsedLogic, appOptions?: AppOptions): void {
    const printProperty = (name, typeNode) =>
        factory.createPropertySignature(undefined, factory.createIdentifier(name), undefined, typeNode)

    const addSelectorTypeHelp = parsedLogic.selectors.filter((s) => s.functionTypes.length > 0).length > 0

    const logicProperties = [
        printProperty('actionCreators', printActionCreators(parsedLogic, appOptions)),
        printProperty('actionKeys', printActionKeys(parsedLogic, appOptions)),
        printProperty('actionTypes', printActionTypes(parsedLogic, appOptions)),
        printProperty('actions', printActions(parsedLogic, appOptions)),
        printProperty('constants', printConstants(parsedLogic)),
        printProperty('defaults', printDefaults(parsedLogic)),
        printProperty('events', printEvents(parsedLogic)),
        printProperty('key', printKey(parsedLogic)),
        printProperty('listeners', printListeners(parsedLogic)),
        printProperty(
            'path',
            factory.createTupleTypeNode(
                parsedLogic.path.map((p) => factory.createLiteralTypeNode(factory.createStringLiteral(p))),
            ),
        ),
        printProperty('pathString', factory.createStringLiteral(parsedLogic.pathString)),
        printProperty('props', printProps(parsedLogic)),
        printProperty('reducer', printReducer(parsedLogic)),
        printProperty('reducerOptions', printReducerOptions(parsedLogic)),
        printProperty('reducers', printReducers(parsedLogic)),
        printProperty('selector', printSelector(parsedLogic)),
        printProperty('selectors', printSelectors(parsedLogic)),
        printProperty('sharedListeners', printSharedListeners(parsedLogic)),
        printProperty('values', printValues(parsedLogic)),
        printProperty('_isKea', factory.createTrue()),
        printProperty('_isKeaWithKey', parsedLogic.keyType ? factory.createTrue() : factory.createFalse()),
        addSelectorTypeHelp
            ? printProperty('__keaTypeGenInternalSelectorTypes', printInternalSelectorTypes(parsedLogic))
            : null,
        Object.keys(parsedLogic.extraActions).length > 0
            ? printProperty('__keaTypeGenInternalReducerActions', printInternalReducerActions(parsedLogic))
            : null,
        Object.keys(parsedLogic.extraInput).length > 0
            ? printProperty('__keaTypeGenInternalExtraInput', printInternalExtraInput(parsedLogic))
            : null,
    ].filter((a) => !!a)

    const logicTypeArguments = getLogicTypeArguments(parsedLogic)

    parsedLogic.interfaceDeclaration = factory.createInterfaceDeclaration(
        undefined,
        [factory.createModifier(SyntaxKind.ExportKeyword)],
        factory.createIdentifier(`${parsedLogic.logicName}Type`),
        logicTypeArguments,
        [
            factory.createHeritageClause(SyntaxKind.ExtendsKeyword, [
                factory.createExpressionWithTypeArguments(factory.createIdentifier('Logic'), undefined),
            ]),
        ],
        logicProperties,
    )
}

// haha
let i = 0
const smiles = ['/', ']', '[', ')', '(', '\\', 'D', '|', 'O']
