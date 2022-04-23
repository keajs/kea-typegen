import { ParsedLogic } from '../types'
import { visit, types } from 'recast'

export const t = types.namedTypes
export const b = types.builders

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
