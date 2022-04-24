import { AppOptions, ParsedLogic } from '../types'
import * as ts from 'typescript'
import { print, visit } from 'recast'
import { runThroughPrettier } from '../print/print'
import * as fs from 'fs'
import * as osPath from 'path'
import { t, b, visitAllKeaCalls, assureImport, getAst } from "./utils";

export function writePaths(appOptions: AppOptions, program: ts.Program, filename: string, parsedLogics: ParsedLogic[]) {
    const { log } = appOptions
    const sourceFile = program.getSourceFile(filename)
    const rawCode = sourceFile.getText()

    const ast = getAst(filename, rawCode)

    let logicPathImportedAs: string | undefined
    let hasImportFromKea = false
    let someOtherPackageImportsPath = false

    // gather information about what is imported
    visit(ast, {
        visitImportDeclaration(path) {
            const isKeaImport =
                path.value.source && t.StringLiteral.check(path.value.source) && path.value.source.value === 'kea'

            if (isKeaImport) {
                hasImportFromKea = true
            }
            for (const specifier of path.value.specifiers) {
                if (isKeaImport) {
                    if (specifier.imported.name === 'path') {
                        logicPathImportedAs = specifier.local.name
                    }
                } else {
                    if (specifier.local.name === 'path') {
                        someOtherPackageImportsPath = true
                    }
                }
            }
            return false
        },
    })

    // add `import { path } from 'kea'` if not yet imported
    const mustImportPath = !!parsedLogics.find((l) => l.inputBuilderArray && !l.hasPathInLogic)
    if (mustImportPath && !logicPathImportedAs) {
        logicPathImportedAs = someOtherPackageImportsPath ? 'logicPath' : 'path'
        assureImport(ast, 'kea', 'path', logicPathImportedAs, hasImportFromKea)
    }

    // find all kea calls, add `path([])` or `path: []` if needed
    visitAllKeaCalls(ast, parsedLogics, filename, ({ path, parsedLogic }) => {
        const stmt = path.node
        const arg = stmt.arguments[0]
        const logicPath = parsedLogic.path

        if (t.ObjectExpression.check(arg)) {
            const pathProperty = arg.properties.find(
                (property) =>
                    t.ObjectProperty.check(property) &&
                    t.Identifier.check(property.key) &&
                    property.key.name === 'path',
            )
            if (!pathProperty) {
                arg.properties = [
                    b.objectProperty(
                        b.identifier('path'),
                        b.arrayExpression(logicPath.map((str) => b.stringLiteral(str))),
                    ),
                    ...arg.properties,
                ]
            }
        } else if (t.ArrayExpression.check(arg)) {
            arg.elements = [
                b.callExpression(b.identifier(logicPathImportedAs), [
                    b.arrayExpression(logicPath.map((str) => b.stringLiteral(str))),
                ]),
                ...arg.elements,
            ]
        }
    })

    const newText = runThroughPrettier(print(ast).code, filename)
    fs.writeFileSync(filename, newText)

    log(`🔥 Path added: ${osPath.relative(process.cwd(), filename)}`)
}
