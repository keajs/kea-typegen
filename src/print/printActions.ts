import {AppOptions, ParsedLogic} from '../types'
import * as ts from 'typescript'
import * as path from 'path'

export function printActions(parsedLogic: ParsedLogic, appOptions?: AppOptions) {
    const cwd = appOptions.logicStartPath ? process.cwd() : process.cwd()

    const pathName = path
        .relative(cwd, parsedLogic.fileName)
        .replace(/^.\//, '')
        .replace(/\.[jt]sx?$/, '')
        .replace(/\//g, '.')

    const toSpaces = (key) => key.replace(/(?:^|\.?)([A-Z])/g, (x, y) => ' ' + y.toLowerCase()).replace(/^ /, '')

    return ts.createTypeLiteralNode(
        parsedLogic.actions.map(({ name, parameters, returnTypeNode }) => {
            return ts.createPropertySignature(
                undefined,
                ts.createIdentifier(name),
                undefined,
                ts.createFunctionTypeNode(
                    undefined,
                    parameters,
                    ts.createParenthesizedType(
                        ts.createTypeLiteralNode([
                            ts.createPropertySignature(
                                undefined,
                                ts.createIdentifier('type'),
                                undefined,
                                ts.createLiteralTypeNode(ts.createStringLiteral(`${toSpaces(name)} (${pathName})`)),
                                undefined,
                            ),
                            ts.createPropertySignature(
                                undefined,
                                ts.createIdentifier('payload'),
                                undefined,
                                returnTypeNode,
                                undefined,
                            ),
                        ]),
                    ),
                ),
                undefined,
            )
        }),
    )
}
