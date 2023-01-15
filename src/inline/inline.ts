import { ParsedLogic } from '../types'
import * as fs from 'fs'
import { nodeToString, runThroughPrettier } from '../print/print'
import { parse, print, visit } from 'recast'
import type { NodePath } from 'ast-types/lib/node-path'
import { t, b, visitAllKeaCalls, getAst, assureImport } from '../write/utils'
import { factory, SyntaxKind } from 'typescript'
import { cleanDuplicateAnyNodes } from '../utils'

export function inlineFiles(
    program,
    appOptions,
    parsedLogics,
): { filesToWrite: number; writtenFiles: number; filesToModify: number } {
    const { log } = appOptions

    const groupedByFile: Record<string, ParsedLogic[]> = {}
    for (const parsedLogic of parsedLogics) {
        if (!groupedByFile[parsedLogic.fileName]) {
            groupedByFile[parsedLogic.fileName] = []
        }
        groupedByFile[parsedLogic.fileName].push(parsedLogic)
    }

    for (const [filename, parsedLogics] of Object.entries(groupedByFile)) {
        const sourceFile = program.getSourceFile(filename)
        const rawCode = sourceFile.getText()

        const ast = getAst(filename, rawCode)

        const parsedLogicTypeNames = new Set<string>(parsedLogics.map((l) => l.logicTypeName))
        const foundLogicTypes = new Map<string, NodePath>()

        let hasImportFromKea = false

        visit(ast, {
            visitTSTypeAliasDeclaration(path): any {
                if (parsedLogicTypeNames.has(path.value.id.name)) {
                    foundLogicTypes.set(path.value.id.name, path)
                }
                return false
            },
            visitImportDeclaration(path) {
                const isKeaImport =
                    path.value.source && t.StringLiteral.check(path.value.source) && path.value.source.value === 'kea'
                if (isKeaImport) {
                    hasImportFromKea = true
                }
                return false
            },
        })

        assureImport(ast, 'kea', 'MakeLogicType', 'MakeLogicType', hasImportFromKea)

        visitAllKeaCalls(ast, parsedLogics, filename, ({ path, parsedLogic }) => {
            path.node.typeParameters = b.tsTypeParameterInstantiation([
                b.tsTypeReference(b.identifier(parsedLogic.logicTypeName)),
            ])
            if (foundLogicTypes.has(parsedLogic.logicTypeName)) {
                const typeAlias: NodePath = foundLogicTypes.get(parsedLogic.logicTypeName)
                if (t.TSTypeAliasDeclaration.check(typeAlias.value)) {
                    typeAlias.value.typeAnnotation = createLogicTypeReference(parsedLogic)
                }
            } else {
                let ptr: NodePath = path
                while (ptr) {
                    if (ptr.parentPath?.value === ast.program.body) {
                        const index = ast.program.body.findIndex((n) => n === ptr.value)
                        ast.program.body = [
                            ...ast.program.body.slice(0, index),
                            b.exportNamedDeclaration(
                                b.tsTypeAliasDeclaration(
                                    b.identifier(parsedLogic.logicTypeName),
                                    createLogicTypeReference(parsedLogic),
                                ),
                            ),
                            ...ast.program.body.slice(index),
                        ]
                    }
                    ptr = ptr.parentPath
                }
            }
        })

        const newText = runThroughPrettier(print(ast).code, filename)
        fs.writeFileSync(filename, newText)
        debugger
    }

    return { filesToWrite: 0, writtenFiles: 0, filesToModify: 0 }
}

export function createLogicTypeReference(parsedLogic: ParsedLogic): ReturnType<typeof b.tsTypeReference> {
    const typeReferenceNode = factory.createTypeReferenceNode(factory.createIdentifier('MakeLogicType'), [
        // values
        factory.createTypeLiteralNode(
            cleanDuplicateAnyNodes(parsedLogic.reducers.concat(parsedLogic.selectors)).map((reducer) => {
                return factory.createPropertySignature(
                    undefined,
                    factory.createIdentifier(reducer.name),
                    undefined,
                    reducer.typeNode,
                )
            }),
        ),
        // actions
        factory.createTypeLiteralNode(
            parsedLogic.actions.map(({ name, parameters, returnTypeNode }) =>
                factory.createPropertySignature(undefined, factory.createIdentifier(name), undefined, returnTypeNode),
            ),
        ),
        // props
        parsedLogic.propsType ||
            factory.createTypeReferenceNode(factory.createIdentifier('Record'), [
                factory.createKeywordTypeNode(SyntaxKind.StringKeyword),
                factory.createKeywordTypeNode(SyntaxKind.UnknownKeyword),
            ]),
    ])

    // Transform Typescript API's AST to a string
    const source = nodeToString(
        factory.createTypeAliasDeclaration(
            [],
            [],
            factory.createIdentifier('githubLogicType'),
            undefined,
            typeReferenceNode,
        ),
    )

    // Convert that string to recast's AST
    const node = getAst(parsedLogic.fileName, source).program.body[0].typeAnnotation
    debugger

    return node
}
