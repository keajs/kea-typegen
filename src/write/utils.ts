import { ParsedLogic } from '../types'
import { visit, types, parse } from 'recast'

export const t = types.namedTypes
export const b = types.builders

import { parseSync } from '@babel/core'

export function getAst(filename: string, source: string): any {
    return parse(source, {
        parser: {
            parse: (source) =>
                parseSync(source, {
                    presets: ['@babel/preset-env', '@babel/preset-react', '@babel/preset-typescript'],
                    filename: filename,
                    parserOpts: {
                        tokens: true, // recast uses this
                    },
                }),
        },
    })
}

export function isKeaCall(path: any): boolean {
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

export function visitAllKeaCalls(
    ast: any,
    parsedLogics: ParsedLogic[],
    filename: string,
    callback: (args: { parsedLogic: ParsedLogic; logicName: string; path: any }) => any,
): void {
    visit(ast, {
        visitCallExpression: function (path) {
            if (!isKeaCall(path)) {
                this.traverse(path)
                return
            }
            const logicName = path.parentPath.value.id?.name
            if (!logicName) {
                console.warn(
                    `❗ Can not visit logic in "${filename}:${JSON.stringify(
                        path.node.loc,
                    )}" because it's not stored as a variable.`,
                )
                return
            }
            const parsedLogic = parsedLogics.find((l) => l.logicName === logicName)
            if (!parsedLogic) {
                console.error(
                    `❗ While trying to visit a logic, could not find logicName "${logicName}" in the list of logicNames (${Object.keys(
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

export function assureImport(ast: any, importFrom: string, importName: string, localName: string, hasImport: boolean) {
    if (hasImport) {
        visit(ast, {
            visitImportDeclaration(path) {
                if (
                    path.value.source &&
                    t.StringLiteral.check(path.value.source) &&
                    path.value.source.value === importFrom
                ) {
                    path.value.specifiers.push(b.importSpecifier(b.identifier(importName), b.identifier(localName)))
                }
                return false
            },
        })
    } else {
        ast.program.body = [
            b.importDeclaration(
                [b.importSpecifier(b.identifier(importName), b.identifier(localName))],
                b.stringLiteral(importFrom),
            ),
            ...ast.program.body,
        ]
    }
}
