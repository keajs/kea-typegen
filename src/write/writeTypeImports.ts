import { AppOptions, ParsedLogic } from '../types'
import * as ts from 'typescript'
import * as osPath from 'path'
import { runThroughPrettier } from '../print/print'
import * as fs from 'fs'

interface TextEdit {
    start: number
    end: number
    text: string
}

function applyTextEdits(source: string, edits: TextEdit[]): string {
    return edits
        .sort((a, b) => b.start - a.start || b.end - a.end)
        .reduce((output, { start, end, text }) => output.slice(0, start) + text + output.slice(end), source)
}

function getImportPath(importDeclaration: ts.ImportDeclaration): string | null {
    return ts.isStringLiteralLike(importDeclaration.moduleSpecifier) ? importDeclaration.moduleSpecifier.text : null
}

function getImportInsertPosition(sourceFile: ts.SourceFile, rawCode: string): number {
    const importDeclarations = sourceFile.statements.filter(ts.isImportDeclaration)
    if (importDeclarations.length > 0) {
        return importDeclarations[importDeclarations.length - 1].getEnd()
    }

    const shebangMatch = rawCode.match(/^#!.*(?:\r?\n|$)/)
    return shebangMatch ? shebangMatch[0].length : 0
}

function getTypeArgumentInsertEnd(callExpression: ts.CallExpression, sourceFile: ts.SourceFile): number {
    const openParenToken = callExpression
        .getChildren(sourceFile)
        .find((child) => child.kind === ts.SyntaxKind.OpenParenToken)

    return openParenToken ? openParenToken.getStart(sourceFile) : callExpression.expression.getEnd()
}

export async function writeTypeImports(
    appOptions: AppOptions,
    program: ts.Program,
    filename: string,
    logicsNeedingImports: ParsedLogic[],
    parsedLogics: ParsedLogic[],
) {
    const { log } = appOptions
    const sourceFile = program.getSourceFile(filename)
    const rawCode = fs.readFileSync(filename, 'utf8')

    if (!sourceFile) {
        throw new Error(`Could not find source file: ${filename}`)
    }

    let importLocation = osPath
        .relative(osPath.dirname(filename), logicsNeedingImports[0].typeFileName)
        .replace(/\.[tj]sx?$/, '')
    if (!importLocation.startsWith('.')) {
        importLocation = `./${importLocation}`
    }

    const desiredImport = `import type { ${parsedLogics.map((l) => l.logicTypeName).join(', ')} } from '${importLocation}'`
    const importDeclarations = sourceFile.statements.filter(ts.isImportDeclaration)
    const matchingImport = importDeclarations
        .find((importDeclaration) => {
            const importPath = getImportPath(importDeclaration)
            return (
                importPath !== null &&
                osPath.resolve(osPath.dirname(filename), importPath) ===
                    osPath.resolve(osPath.dirname(filename), importLocation)
            )
        })

    const edits: TextEdit[] = []
    if (matchingImport) {
        edits.push({
            start: matchingImport.getStart(sourceFile),
            end: matchingImport.getEnd(),
            text: desiredImport,
        })
    } else {
        const insertPos = getImportInsertPosition(sourceFile, rawCode)
        const hasExistingImports = importDeclarations.length > 0
        const importText = hasExistingImports
            ? `\n${desiredImport}${rawCode.slice(insertPos, insertPos + 1) === '\n' ? '' : '\n'}`
            : `${desiredImport}\n`

        edits.push({
            start: insertPos,
            end: insertPos,
            text: importText,
        })
    }

    for (const parsedLogic of logicsNeedingImports) {
        const callExpression = parsedLogic.node.parent
        if (!ts.isCallExpression(callExpression)) {
            continue
        }

        const typeArgumentStart = callExpression.expression.getEnd()
        const typeArgumentEnd = callExpression.typeArguments
            ? getTypeArgumentInsertEnd(callExpression, sourceFile)
            : typeArgumentStart

        edits.push({
            start: typeArgumentStart,
            end: typeArgumentEnd,
            text: `<${parsedLogic.logicTypeName}>`,
        })
    }

    const newText = await runThroughPrettier(applyTextEdits(rawCode, edits), filename)
    fs.writeFileSync(filename, newText)

    log(`🔥 Import added: ${osPath.relative(process.cwd(), filename)}`)
}
