import * as ts from 'typescript'
import * as fs from 'fs'
import * as path from 'path'
import { AppOptions, ParsedLogic } from '../types'
import { printActions } from './printActions'
import { printReducers } from './printReducers'
import { printReducer } from './printReducer'
import { printSelector } from './printSelector'
import { printSelectors } from './printSelectors'
import { printValues } from './printValues'
import { printSelectorTypeHelp } from './printSelectorTypeHelp'

export function printToFiles(appOptions: AppOptions, parsedLogics: ParsedLogic[]) {
    const { log } = appOptions

    const groupedByFile: Record<string, ParsedLogic[]> = {}
    parsedLogics.forEach((parsedLogic) => {
        if (!groupedByFile[parsedLogic.fileName]) {
            groupedByFile[parsedLogic.fileName] = []
        }
        groupedByFile[parsedLogic.fileName].push(parsedLogic)
    })

    let writtenFiles = 0
    let filesToWrite = 0

    Object.entries(groupedByFile).forEach(([fileName, parsedLogics]) => {
        const output = parsedLogics.map((l) => parsedLogicToTypeString(l, appOptions)).join('\n\n')
        fileName = fileName.replace(/\.[tj]sx?$/, '.type.ts')
        const finalOutput = `// Auto-generated with kea-typegen. DO NOT EDIT!\n\n${output}`

        let existingOutput

        try {
            existingOutput = fs.readFileSync(fileName)
        } catch (error) {}

        if (existingOutput?.toString() !== finalOutput) {
            filesToWrite += 1
            if (appOptions.write) {
                fs.writeFileSync(fileName, finalOutput)
                writtenFiles += 1
                log(`!! Writing: ${path.relative(process.cwd(), fileName)}`)
            } else {
                log(`:${smiles[i++ % smiles.length]} Would write: ${path.relative(process.cwd(), fileName)}`)
            }
        } else {
            if (appOptions.verbose) {
                log(`-- Unchanged: ${path.relative(process.cwd(), fileName)}`)
            }
        }
    })

    if (filesToWrite > 0 || (appOptions.verbose && Object.keys(groupedByFile).length > 0)) {
        log('')
    }

    if (writtenFiles === 0) {
        log(`-> Nothing was written to disk`)
        if (filesToWrite > 0) {
            log(`-> Run with "--write" to save types to disk!`)
        }
    } else if (writtenFiles > 0) {
        log(`!> Wrote ${writtenFiles} file${writtenFiles === 1 ? '' : 's'}!`)
    }
}

export function parsedLogicToTypeString(parsedLogic: ParsedLogic, appOptions?: AppOptions) {
    const logicType = printLogicType(parsedLogic, appOptions)
    const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed })
    const sourceFile = ts.createSourceFile('logic.ts', '', ts.ScriptTarget.Latest, false, ts.ScriptKind.TS)
    return printer.printNode(ts.EmitHint.Unspecified, logicType, sourceFile)
}

export function printLogicType(parsedLogic: ParsedLogic, appOptions?: AppOptions) {
    const printProperty = (name, typeNode) =>
        ts.createPropertySignature(undefined, ts.createIdentifier(name), undefined, typeNode, undefined)

    const addSelectorTypeHelp = parsedLogic.selectors.filter((s) => s.functionTypes.length > 0).length > 0

    let cwd = process.cwd()

    if (appOptions?.logicStartPath) {
        cwd = path.resolve(cwd, appOptions.logicStartPath)
    }

    const pathString = path
        .relative(cwd, parsedLogic.fileName)
        .replace(/^.\//, '')
        .replace(/\.[jt]sx?$/, '')
        .replace(/\//g, '.')

    return ts.createInterfaceDeclaration(
        undefined,
        [ts.createModifier(ts.SyntaxKind.ExportKeyword)],
        ts.createIdentifier(`${parsedLogic.logicName}Type`),
        parsedLogic.logicTypeArguments.map((text) =>
            ts.createTypeParameterDeclaration(ts.createIdentifier(text), undefined),
        ),
        undefined,
        [
            // TODO
            printProperty('key', ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)),
            printProperty('actionCreators', printActions(parsedLogic, appOptions)),
            // TODO
            printProperty(
                'actionKeys',
                ts.createTypeReferenceNode(ts.createIdentifier('Record'), [
                    ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                    ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                ]),
            ),
            // TODO
            printProperty(
                'actionTypes',
                ts.createTypeReferenceNode(ts.createIdentifier('Record'), [
                    ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                    ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                ]),
            ),
            printProperty('actions', printActions(parsedLogic, appOptions)),
            printProperty(
                'cache',
                ts.createTypeReferenceNode(ts.createIdentifier('Record'), [
                    ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                    ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                ]),
            ),
            // TODO
            printProperty('connections', ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)),
            // TODO
            printProperty('constants', ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)),
            // TODO
            printProperty('defaults', ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)),
            // TODO
            printProperty('events', ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)),
            // inputs
            // listeners
            printProperty(
                'path',
                ts.createTupleTypeNode(
                    pathString.split('.').map((p) => ts.createLiteralTypeNode(ts.createStringLiteral(p))),
                ),
            ),
            printProperty('pathString', ts.createStringLiteral(pathString)),
            // TODO
            printProperty('propTypes', ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)),
            printProperty(
                'props',
                ts.createTypeReferenceNode(ts.createIdentifier('Record'), [
                    ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                    ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                ]),
            ),
            printProperty('reducer', printReducer(parsedLogic)),
            // TODO
            printProperty('reducerOptions', ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)),
            printProperty('reducers', printReducers(parsedLogic)),
            printProperty('selector', printSelector(parsedLogic)),
            printProperty('selectors', printSelectors(parsedLogic)),
            // sharedListeners
            printProperty('values', printValues(parsedLogic)),
            printProperty('_isKea', ts.createTrue()),
            // _isKeaWithKey,
            addSelectorTypeHelp ? printProperty('__selectorTypeHelp', printSelectorTypeHelp(parsedLogic)) : null,
        ].filter((a) => a),
    )
}

// haha
let i = 0
const smiles = ['/', ']', '[', ')', '(', '\\', 'D', '|', 'O']
