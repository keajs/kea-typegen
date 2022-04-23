import { AppOptions, ParsedLogic } from '../types'
import * as ts from 'typescript'
import * as fs from 'fs'
import { runThroughPrettier } from '../print/print'
import * as osPath from 'path'
import { parse, print, visit, types } from 'recast'

const t = types.namedTypes
const b = types.builders

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

    const ast = parse(rawCode, {
        parser: require('recast/parsers/typescript'),
    })

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

    // find all kea calls, add `<logicType<a,b>>` type parameters if needed
    visitAllKeaCalls(ast, logicsNeedingImports, filename, ({ path, parsedLogic }) => {
        const { logicTypeName, typeReferencesInLogicInput } = parsedLogic

        path.node.typeParameters = b.tsTypeParameterInstantiation([
            b.tsTypeReference(
                b.identifier(logicTypeName),
                typeReferencesInLogicInput.size > 0 ? b.tsTypeParameterInstantiation(
                    [...typeReferencesInLogicInput.values()]
                        .sort()
                        .map((type) => b.tsTypeReference(b.identifier(type))),
                ) : null,
            ),
        ])
    })

    const newText = runThroughPrettier(print(ast).code, filename)
    fs.writeFileSync(filename, newText)

    log(`🔥 Import added: ${osPath.relative(process.cwd(), filename)}`)
}

export function writePaths(appOptions: AppOptions, program: ts.Program, filename: string, parsedLogics: ParsedLogic[]) {
    const { log } = appOptions
    const sourceFile = program.getSourceFile(filename)
    const rawCode = sourceFile.getText()

    const ast = parse(rawCode, {
        parser: require('recast/parsers/typescript'),
    })

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
        if (hasImportFromKea) {
            visit(ast, {
                visitImportDeclaration(path) {
                    if (
                        path.value.source &&
                        t.StringLiteral.check(path.value.source) &&
                        path.value.source.value === 'kea'
                    ) {
                        path.value.specifiers.push(
                            b.importSpecifier(b.identifier('path'), b.identifier(logicPathImportedAs)),
                        )
                    }
                    return false
                },
            })
        } else {
            ast.program.body = [
                b.importDeclaration(
                    [b.importSpecifier(b.identifier('path'), b.identifier(logicPathImportedAs))],
                    b.stringLiteral('kea'),
                ),
                ...ast.program.body,
            ]
        }
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

function isKeaCall(path: any): boolean {
    const stmt = path.node
    return (
        t.Identifier.check(stmt.callee) &&
        stmt.callee.name === 'kea' &&
        stmt.arguments[0] &&
        (t.ObjectExpression.check(stmt.arguments[0]) || t.ArrayExpression.check(stmt.arguments[0])) &&
        path.parentPath &&
        t.VariableDeclarator.check(path.parentPath.value) &&
        t.Identifier.check(path.parentPath.value.id)
    )
}

function visitAllKeaCalls(
    ast: any,
    parsedLogics: ParsedLogic[],
    filename: string,
    callback: (args: { parsedLogic: ParsedLogic; logicName: string; path: any }) => any,
): void {
    visit(ast, {
        visitCallExpression: function (path) {
            if (!isKeaCall(path)) {
                this.traverse(path)
            }
            const logicName = path.parentPath.value.id.name
            if (!logicName) {
                console.warn(
                    `[KEA-TYPEGEN] Can not add path to logic in "${filename}:${path.node.loc.start}" because it's not stored as a variable.`,
                )
                return
            }
            const parsedLogic = parsedLogics.find((l) => l.logicName === logicName)
            if (!parsedLogic) {
                console.error(
                    `[KEA-TYPEGEN] While trying to add a path, could not find logicName "${logicName}" in the list of logicNames (${Object.keys(
                        parsedLogics.map((l) => l.logicName),
                    ).join(', ')}) in the file: ${filename}`,
                )
                return
            }
            callback.bind(this)({ logicName, parsedLogic, path })
            return false
        },
    })
}
