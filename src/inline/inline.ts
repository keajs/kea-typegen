import { ParsedLogic } from '../types'
import * as fs from 'fs'
import {  runThroughPrettier } from '../print/print'
import { print, visit } from 'recast'
import type { NodePath } from 'ast-types/lib/node-path'
import { t, b, visitAllKeaCalls, getAst } from '../write/utils'

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

        // find all keaType calls, add interface declaration if missing
        visit(ast, {
            visitTSTypeAliasDeclaration(path): any {
                if (parsedLogicTypeNames.has(path.value.id.name)) {
                    foundLogicTypes.set(path.value.id.name, path)
                }
                return false
            },
        })

        // find all kea calls, add `<logicType>` type parameters if needed
        visitAllKeaCalls(ast, parsedLogics, filename, ({ path, parsedLogic }) => {
            if (parsedLogic.logicTypeName && foundLogicTypes.has(parsedLogic.logicTypeName)) {
                const typeAlias: NodePath = foundLogicTypes.get(parsedLogic.logicTypeName)
                if (t.TSTypeAliasDeclaration.check(typeAlias.value)) {
                    typeAlias.value.typeAnnotation = createLogicTypeReference(parsedLogic)
                }
            }
            if (parsedLogic.logicTypeName && !foundLogicTypes.has(parsedLogic.logicTypeName)) {
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
    }

    return { filesToWrite: 0, writtenFiles: 0, filesToModify: 0 }
}

export function createLogicTypeReference(parsedLogic: ParsedLogic): ReturnType<typeof b.tsTypeReference> {
    const node = b.tsTypeReference(b.identifier('LogicType'))
    debugger
    return node
}
