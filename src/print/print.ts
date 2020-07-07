import * as ts from 'typescript'
import * as fs from 'fs'
import { ParsedLogic } from '../types'
import { printActions } from './printActions'
import { printReducers } from './printReducers'
import { printReducer } from './printReducer'
import { printSelector } from './printSelector'
import { printSelectors } from './printSelectors'
import { printValues } from './printValues'

export function printToFiles(parsedLogics: ParsedLogic[], verbose: boolean = false) {
    const groupedByFile: Record<string, ParsedLogic[]> = {}
    parsedLogics.forEach((parsedLogic) => {
        if (!groupedByFile[parsedLogic.fileName]) {
            groupedByFile[parsedLogic.fileName] = []
        }
        groupedByFile[parsedLogic.fileName].push(parsedLogic)
    })

    Object.entries(groupedByFile).forEach(([fileName, parsedLogics]) => {
        const output = parsedLogics.map(parsedLogicToTypeString).join('\n\n')
        fileName = fileName.replace(/\.[tj]sx?$/, '.type.ts')
        if (verbose) {
            console.log(`Writing: ${fileName}`)
        }
        fs.writeFileSync(fileName, output)
    })
}

export function parsedLogicToTypeString(parsedLogic: ParsedLogic) {
    const logicType = printLogicType(parsedLogic)
    const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed })
    const sourceFile = ts.createSourceFile('logic.ts', '', ts.ScriptTarget.Latest, false, ts.ScriptKind.TS)
    return printer.printNode(ts.EmitHint.Unspecified, logicType, sourceFile)
}

export function printLogicType(parsedLogic: ParsedLogic) {
    const printProperty = (name, typeNode) =>
        ts.createPropertySignature(undefined, ts.createIdentifier(name), undefined, typeNode, undefined)

    return ts.createInterfaceDeclaration(
        undefined,
        [ts.createModifier(ts.SyntaxKind.ExportKeyword)],
        ts.createIdentifier(`${parsedLogic.logicName}Type`),
        undefined,
        undefined,
        [
            printProperty('actionCreators', printActions(parsedLogic)),
            // actionKeys
            printProperty('actions', printActions(parsedLogic)),
            // build
            // cache
            // connections
            // constants
            // defaults
            // events
            // extend
            // inputs
            // listeners
            // mount
            // path
            // pathString
            // propTypes
            // props
            printProperty('reducer', printReducer(parsedLogic)),
            // reducerOptions
            printProperty('reducers', printReducers(parsedLogic)),
            printProperty('selector', printSelector(parsedLogic)),
            printProperty('selectors', printSelectors(parsedLogic)),
            // sharedListeners
            printProperty('values', printValues(parsedLogic)),
            // wrap
            // _isKea
            // _isKeaWithKey
        ],
    )
}
