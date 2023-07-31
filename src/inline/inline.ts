import { AppOptions, ParsedLogic } from '../types'
import * as fs from 'fs'
import { getShouldIgnore, nodeToString, runThroughPrettier } from '../print/print'
import { print, visit } from 'recast'
import type { NodePath } from 'ast-types/lib/node-path'
import type { namedTypes } from 'ast-types/gen/namedTypes'
import { t, b, visitAllKeaCalls, getAst, assureImport } from '../write/utils'
import { factory, SyntaxKind, Program } from 'typescript'
import { cleanDuplicateAnyNodes } from '../utils'
import { printInternalExtraInput } from '../print/printInternalExtraInput'
import { printInternalSelectorTypes } from '../print/printInternalSelectorTypes'
import { printInternalReducerActions } from '../print/printInternalReducerActions'
import * as ts from 'typescript'
import * as path from 'path'

export function inlineFiles(
    program: Program,
    appOptions: AppOptions,
    parsedLogics: ParsedLogic[],
): { filesToWrite: number; writtenFiles: number; filesToModify: number } {
    const groupedByFile: Record<string, ParsedLogic[]> = {}
    for (const parsedLogic of parsedLogics) {
        if (!groupedByFile[parsedLogic.fileName]) {
            groupedByFile[parsedLogic.fileName] = []
        }
        groupedByFile[parsedLogic.fileName].push(parsedLogic)
    }

    for (const [filename, parsedLogics] of Object.entries(groupedByFile)) {
        try {
            const sourceFile = program.getSourceFile(filename)
            const rawCode = sourceFile.getText()

            const ast = getAst(filename, rawCode)

            const parsedLogicTypeNames = new Set<string>(parsedLogics.map((l) => l.logicTypeName))
            const foundLogicTypes = new Map<string, NodePath>()

            let hasImportFromKea = false
            let foundKeaLogicTypeImport = false
            const importedVariables = new Set<string>()

            visit(ast, {
                visitTSTypeAliasDeclaration(path): any {
                    if (parsedLogicTypeNames.has(path.value.id.name)) {
                        foundLogicTypes.set(path.value.id.name, path)
                    }
                    return false
                },
                visitImportDeclaration(path) {
                    const isKeaImport =
                        path.value.source &&
                        t.StringLiteral.check(path.value.source) &&
                        path.value.source.value === 'kea'

                    if (isKeaImport) {
                        hasImportFromKea = true
                        for (const specifier of path.value.specifiers) {
                            if (specifier.imported.name === 'KeaLogicType') {
                                foundKeaLogicTypeImport = true
                            }
                        }
                    }

                    // remove non-inline imports from external loginType.ts files
                    for (const specifier of path.value.specifiers) {
                        if (specifier.imported?.name && parsedLogicTypeNames.has(specifier.imported.name)) {
                            path.value.specifiers = path.value.specifiers.filter((s) => s !== specifier)
                            if (path.value.specifiers.length === 0) {
                                path.prune()
                            }
                        } else {
                            importedVariables.add(specifier.local?.name ?? specifier.imported?.name)
                        }
                    }

                    return false
                },
            })

            if (!foundKeaLogicTypeImport) {
                assureImport(ast, 'kea', 'KeaLogicType', 'KeaLogicType', hasImportFromKea)
            }

            visitAllKeaCalls(ast, parsedLogics, filename, ({ path, parsedLogic }) => {
                path.node.typeParameters = b.tsTypeParameterInstantiation([
                    b.tsTypeReference(b.identifier(parsedLogic.logicTypeName)),
                ])
                if (foundLogicTypes.has(parsedLogic.logicTypeName)) {
                    const typeAlias: NodePath = foundLogicTypes.get(parsedLogic.logicTypeName)
                    typeAlias.parentPath.value.comments = createLogicTypeComments(parsedLogic)
                    if (t.TSTypeAliasDeclaration.check(typeAlias.value)) {
                        typeAlias.value.typeAnnotation = createLogicTypeReference(
                            program,
                            appOptions,
                            parsedLogic,
                            importedVariables,
                            ast.program.body,
                        )
                    }
                } else {
                    let ptr: NodePath = path
                    while (ptr) {
                        if (ptr.parentPath?.value === ast.program.body) {
                            const index = ast.program.body.findIndex((n) => n === ptr.value)
                            const logicTypeNode = b.exportNamedDeclaration(
                                b.tsTypeAliasDeclaration(
                                    b.identifier(parsedLogic.logicTypeName),
                                    createLogicTypeReference(
                                        program,
                                        appOptions,
                                        parsedLogic,
                                        importedVariables,
                                        ast.program.body,
                                    ),
                                ),
                            )
                            logicTypeNode.comments = createLogicTypeComments(parsedLogic)
                            ast.program.body = [
                                ...ast.program.body.slice(0, index + 1),
                                logicTypeNode,
                                ...ast.program.body.slice(index + 1),
                            ]
                        }
                        ptr = ptr.parentPath
                    }
                }
            })

            const newText = runThroughPrettier(print(ast).code, filename)
            fs.writeFileSync(filename, newText)
        } catch (e) {
            console.error(`Error updating logic types in ${filename}`)
            console.error(e)
        }
    }

    return { filesToWrite: 0, writtenFiles: 0, filesToModify: 0 }
}

export function createLogicTypeReference(
    program: Program,
    appOptions: AppOptions,
    parsedLogic: ParsedLogic,
    importedVariables: Set<string>,
    body: namedTypes.Program['body'],
): ReturnType<typeof b.tsTypeReference> {
    let typeReferenceNode: ts.TypeNode = factory.createTypeReferenceNode(factory.createIdentifier('KeaLogicType'), [
        factory.createTypeLiteralNode(
            [
                // actions
                factory.createPropertySignature(
                    undefined,
                    factory.createIdentifier('actions'),
                    undefined,
                    factory.createTypeLiteralNode(
                        [...parsedLogic.actions]
                            .sort((a, b) => a.name.localeCompare(b.name))
                            .map(({ name, parameters, returnTypeNode }) =>
                                factory.createPropertySignature(
                                    undefined,
                                    factory.createIdentifier(name),
                                    undefined,
                                    factory.createFunctionTypeNode(undefined, parameters, returnTypeNode),
                                ),
                            ),
                    ),
                ),
                // values
                factory.createPropertySignature(
                    undefined,
                    factory.createIdentifier('values'),
                    undefined,
                    factory.createTypeLiteralNode(
                        cleanDuplicateAnyNodes(parsedLogic.reducers.concat(parsedLogic.selectors))
                            .sort((a, b) => a.name.localeCompare(b.name))
                            .map((reducer) =>
                                factory.createPropertySignature(
                                    undefined,
                                    factory.createIdentifier(reducer.name),
                                    undefined,
                                    reducer.typeNode,
                                ),
                            ),
                    ),
                ),
                // props
                parsedLogic.propsType
                    ? factory.createPropertySignature(
                          undefined,
                          factory.createIdentifier('props'),
                          undefined,
                          parsedLogic.propsType ||
                              factory.createTypeReferenceNode(factory.createIdentifier('Record'), [
                                  factory.createKeywordTypeNode(SyntaxKind.StringKeyword),
                                  factory.createKeywordTypeNode(SyntaxKind.UnknownKeyword),
                              ]),
                      )
                    : undefined,

                parsedLogic.selectors.filter((s) => s.functionTypes.length > 0).length > 0
                    ? factory.createPropertySignature(
                          undefined,
                          factory.createIdentifier('__keaTypeGenInternalSelectorTypes'),
                          undefined,
                          printInternalSelectorTypes(parsedLogic),
                      )
                    : null,

                Object.keys(parsedLogic.extraActions).length > 0
                    ? factory.createPropertySignature(
                          undefined,
                          factory.createIdentifier('__keaTypeGenInternalReducerActions'),
                          undefined,
                          printInternalReducerActions(parsedLogic),
                      )
                    : null,
                Object.keys(parsedLogic.extraInput).length > 0
                    ? factory.createPropertySignature(
                          undefined,
                          factory.createIdentifier('__keaTypeGenInternalExtraInput'),
                          undefined,
                          printInternalExtraInput(parsedLogic),
                      )
                    : null,
            ].filter((a) => !!a),
        ),
    ])

    if (Object.keys(parsedLogic.extraLogicFields).length > 0) {
        typeReferenceNode = factory.createIntersectionTypeNode([
            typeReferenceNode,
            factory.createTypeLiteralNode(
                Object.entries(parsedLogic.extraLogicFields).map(([key, field]) =>
                    factory.createPropertySignature(undefined, factory.createIdentifier(key), undefined, field),
                ),
            ),
        ])
    }

    if (Object.keys(parsedLogic.typeReferencesToImportFromFiles).length > 0) {
        const shouldIgnore = getShouldIgnore(program, appOptions)
        const requiredImports = Object.entries(parsedLogic.typeReferencesToImportFromFiles)
            .map(([file, list]): [string, string[]] => [file, [...list].filter((key) => !importedVariables.has(key))])
            .filter(([file, list]) => list.length > 0 && !shouldIgnore(file) && file !== parsedLogic.fileName)

        for (const [file, list] of requiredImports) {
            let relativePath = path.relative(path.dirname(parsedLogic.fileName), file)
            relativePath = relativePath.replace(/\.tsx?$/, '')
            if (!relativePath.startsWith('.')) {
                relativePath = `./${relativePath}`
            }

            let importDeclaration = body.find(
                (node) => t.ImportDeclaration.check(node) && node.source.value === relativePath,
            ) as namedTypes.ImportDeclaration | undefined
            if (importDeclaration) {
                importDeclaration.specifiers.push(b.importSpecifier(b.identifier(list[0])))
            } else {
                importDeclaration = b.importDeclaration(
                    list.map((key) => b.importSpecifier(b.identifier(key))),
                    b.stringLiteral(relativePath),
                )
                let lastIndex = -1
                for (let i = 0; i < body.length; i++) {
                    if (t.ImportDeclaration.check(body[i])) {
                        lastIndex = i
                    }
                }
                body.splice(lastIndex + 1, 0, importDeclaration)
            }
        }
    }

    // Transform Typescript API's AST to a string
    let source: string = ''
    try {
        source = nodeToString(
            factory.createTypeAliasDeclaration(
                [],
                [],
                factory.createIdentifier(parsedLogic.logicTypeName),
                undefined,
                typeReferenceNode,
            ),
        )
    } catch (e) {
        console.error(`Error emitting logic type ${parsedLogic.logicTypeName} to string`)
        console.error(e)
        debugger
    }

    // Convert that string to recast's AST
    const node = getAst(parsedLogic.fileName, source).program.body[0].typeAnnotation

    return node
}

function createLogicTypeComments(parsedLogic: ParsedLogic): namedTypes.CommentLine[] {
    return [
        {
            type: 'CommentLine',
            value: ` This is an auto-generated type for the logic "${parsedLogic.logicName}".`,
            leading: true,
        },
    ]
}
