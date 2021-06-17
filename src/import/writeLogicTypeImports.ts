import { AppOptions, ParsedLogic } from '../types'
import * as ts from 'typescript'
import { isKeaCall } from '../utils'
import * as fs from 'fs'
import { runThroughPrettier } from '../print/print'
import * as diff from 'diff'
import * as path from 'path'

// This is an unfortunate workaround. The TS compiler strips all
// whitespace. This uses "jsdiff" to add them back.
// If it turns out to be flaky, it might be worth rewriting to babel.
// https://github.com/microsoft/TypeScript/issues/843#issuecomment-555932858
function addBackNewlines(oldText: string, newText: string) {
    const patch = diff.parsePatch(diff.createPatch('file', oldText, newText, '', ''))
    const hunks = patch[0].hunks
    for (let i = 0; i < hunks.length; ++i) {
        let lineOffset = 0
        const hunk = hunks[i]
        hunk.lines = hunk.lines.map((line) => {
            if (line === '-') {
                lineOffset++
                return ' '
            }
            return line
        })
        hunk.newLines += lineOffset
        for (let j = i + 1; j < hunks.length; ++j) {
            hunks[j].newStart += lineOffset
        }
    }
    return diff.applyPatch(oldText, patch)
}

export function writeLogicTypeImports(
    appOptions: AppOptions,
    program: ts.Program,
    filename: string,
    parsedLogics: ParsedLogic[],
    allParsedLogics: ParsedLogic[],
) {
    const { log } = appOptions
    const sourceFile = program.getSourceFile(filename)
    const checker = program.getTypeChecker()

    const parsedLogicMapByNode = new Map<ts.Node, ParsedLogic>()
    for (const parsedLogic of parsedLogics) {
        parsedLogicMapByNode.set(parsedLogic.node, parsedLogic)
    }

    let importLocation = path.relative(path.dirname(filename), parsedLogics[0].typeFileName).replace(/\.[tj]sx?$/, '')
    if (!importLocation.startsWith('.')) {
        importLocation = `./${importLocation}`
    }
    const createImportDeclaration = () =>
        ts.createImportDeclaration(
            undefined,
            undefined,
            ts.createImportClause(
                undefined,
                ts.createNamedImports(
                    allParsedLogics.map((l) =>
                        ts.createImportSpecifier(undefined, ts.createIdentifier(l.logicTypeName)),
                    ),
                ),
            ),
            ts.createLiteral(importLocation),
        )

    const transformer = <T extends ts.Node>(context: ts.TransformationContext) => {
        return (rootNode: T) => {
            function visit(node: ts.Node): ts.Node {
                node = ts.visitEachChild(node, visit, context)

                if (
                    ts.isCallExpression(node) &&
                    isKeaCall(node.expression, checker) &&
                    parsedLogicMapByNode.has(node.expression)
                ) {
                    const { logicTypeName, localTypes } = parsedLogicMapByNode.get(node.expression)
                    return ts.createCall(
                        node.expression,
                        [
                            ts.createTypeReferenceNode(
                                ts.createIdentifier(logicTypeName),
                                localTypes.size > 0
                                    ? [...localTypes.values()].map((type) =>
                                          ts.createTypeReferenceNode(ts.createIdentifier(type), undefined),
                                      )
                                    : undefined,
                            ),
                        ],
                        node.arguments,
                    )
                }

                if (ts.isSourceFile(node)) {
                    let foundImport = false
                    let changedImport = false
                    let newStatements = sourceFile.statements.map((node: ts.Statement) => {
                        if (ts.isImportDeclaration(node)) {
                            // Warning: We can not rely on "symbol" leading us to the direct file
                            // (bypassing path resolution), because the symbol will be undefined if the
                            // type is fresh and the AST is not loaded yet. This leads to endless loops.
                            // // const symbol = checker.getSymbolAtLocation(node.moduleSpecifier)
                            // // const typeFile = symbol?.getDeclarations()?.[0]?.getSourceFile().fileName

                            // Warning: This simpler path resolution won't work with aliases.
                            const moduleFile = node.moduleSpecifier.getText().split(/['"]/).join('')

                            if (
                                path.resolve(path.dirname(filename), moduleFile) ===
                                path.resolve(path.dirname(filename), importLocation)
                            ) {
                                foundImport = true
                                const bindings = node.importClause.namedBindings
                                if (ts.isNamedImports(bindings)) {
                                    const oldString = bindings.elements
                                        .map((e) => e.getText())
                                        .sort()
                                        .join(',')
                                    const newString = parsedLogics
                                        .map((l) => l.logicTypeName)
                                        .sort()
                                        .join(',')
                                    if (oldString !== newString) {
                                        changedImport = true
                                        return createImportDeclaration()
                                    }
                                }
                            }
                        }
                        return node
                    })
                    if (!foundImport) {
                        newStatements = [
                            ...sourceFile.statements.filter((node) => ts.isImportDeclaration(node)),
                            createImportDeclaration(),
                            ...sourceFile.statements.filter((node) => !ts.isImportDeclaration(node)),
                        ]
                    }
                    if (!foundImport || changedImport) {
                        return ts.updateSourceFileNode(sourceFile, newStatements)
                    }
                }

                return node
            }
            return ts.visitNode(rootNode, visit)
        }
    }

    const printer: ts.Printer = ts.createPrinter()
    const result: ts.TransformationResult<ts.SourceFile> = ts.transform<ts.SourceFile>(sourceFile, [transformer])

    const transformedSourceFile: ts.SourceFile = result.transformed[0]
    const newContent = printer.printFile(transformedSourceFile)
    const newText = runThroughPrettier(newContent, filename)
    result.dispose()

    const oldText = sourceFile.getText()
    const newestText = addBackNewlines(oldText, newText)

    fs.writeFileSync(filename, newestText)

    log(`🔥 Import added: ${path.relative(process.cwd(), filename)}`)
}
