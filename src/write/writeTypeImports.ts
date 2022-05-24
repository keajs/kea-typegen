import { AppOptions, ParsedLogic } from '../types'
import * as ts from 'typescript'
import { print, visit } from 'recast'
import * as osPath from 'path'
import { runThroughPrettier } from '../print/print'
import * as fs from 'fs'
import { t, b, visitAllKeaCalls, getAst } from './utils'

export function writeTypeImports(
    appOptions: AppOptions,
    program: ts.Program,
    filename: string,
    logicsNeedingImports: ParsedLogic[],
    parsedLogics: ParsedLogic[],
) {
    const { log } = appOptions
    const sourceFile = program.getSourceFile(filename)
    const rawCode = sourceFile.getText()

    const ast = getAst(filename, rawCode)

    let importLocation = osPath
        .relative(osPath.dirname(filename), logicsNeedingImports[0].typeFileName)
        .replace(/\.[tj]sx?$/, '')
    if (!importLocation.startsWith('.')) {
        importLocation = `./${importLocation}`
    }

    // add import if missing
    let foundImport = false
    visit(ast, {
        visitImportDeclaration(path) {
            const importPath =
                path.value.source && t.StringLiteral.check(path.value.source) ? path.value.source.value : null

            if (
                t.ImportDeclaration.check(path.value) &&
                importPath &&
                osPath.resolve(osPath.dirname(filename), importPath) ===
                    osPath.resolve(osPath.dirname(filename), importLocation)
            ) {
                foundImport = true
                path.value.importKind = 'type'
                path.value.specifiers = parsedLogics.map((l) =>
                    b.importSpecifier(b.identifier(l.logicTypeName), b.identifier(l.logicTypeName)),
                )
            }
            return false
        },
    })

    if (!foundImport) {
        visit(ast, {
            visitProgram(path) {
                path.value.body = [
                    ...path.value.body.filter((n) => t.ImportDeclaration.check(n)),
                    b.importDeclaration(
                        parsedLogics.map((l) =>
                            b.importSpecifier(b.identifier(l.logicTypeName), b.identifier(l.logicTypeName)),
                        ),
                        b.stringLiteral(importLocation),
                        'type',
                    ),
                    ...path.value.body.filter((n) => !t.ImportDeclaration.check(n)),
                ]
                return false
            },
        })
    }

    // find all kea calls, add `<logicType>` type parameters if needed
    visitAllKeaCalls(ast, logicsNeedingImports, filename, ({ path, parsedLogic }) => {
        path.node.typeParameters = b.tsTypeParameterInstantiation([
            b.tsTypeReference(b.identifier(parsedLogic.logicTypeName)),
        ])
    })

    const newText = runThroughPrettier(print(ast).code, filename)
    fs.writeFileSync(filename, newText)

    log(`🔥 Import added: ${osPath.relative(process.cwd(), filename)}`)
}
