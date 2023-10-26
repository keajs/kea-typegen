import { AppOptions, ParsedLogic } from '../types'
import * as ts from 'typescript'
import { print, visit } from 'recast'
import { runThroughPrettier } from '../print/print'
import * as fs from 'fs'
import * as osPath from 'path'
import { t, b, visitAllKeaCalls, assureImport, getAst } from "./utils";

const supportedProperties = {
    props: 'kea',
    key: 'kea',
    path: 'kea',
    connect: 'kea',
    actions: 'kea',
    defaults: 'kea',
    loaders: 'kea-loaders',
    forms: 'kea-forms',
    subscriptions: 'kea-subscriptions',
    windowValues: 'kea-window-values',
    reducers: 'kea',
    selectors: 'kea',
    sharedListeners: 'kea',
    thunks: 'kea-thunk',
    listeners: 'kea',
    start: ['kea-saga', 'saga'],
    stop: ['kea-saga', 'cancelled'],
    saga: 'kea-saga',
    workers: 'kea-saga',
    takeEvery: 'kea-saga',
    takeLatest: 'kea-saga',
    actionToUrl: 'kea-router',
    urlToAction: 'kea-router',
    events: 'kea',
}

export function convertToBuilders(
    appOptions: AppOptions,
    program: ts.Program,
    filename: string,
    parsedLogics: ParsedLogic[],
) {
    const { log } = appOptions
    const sourceFile = program.getSourceFile(filename)
    const rawCode = sourceFile.getText()

    const ast = getAst(filename, rawCode)

    // gather information about what is imported
    const imports: Record<string, [string, string][]> = {}
    visit(ast, {
        visitImportDeclaration(path) {
            const importFrom =
                path.value.source && t.StringLiteral.check(path.value.source) ? path.value.source.value : null
            if (!importFrom) {
                return false
            }
            imports[importFrom] ??= []
            for (const specifier of path.value.specifiers) {
                imports[importFrom].push([specifier.imported?.name, specifier.local.name])
            }
            return false
        },
    })

    // find all kea calls that need conversion
    const neededImports: Record<string, [string, string][]> = {}
    visitAllKeaCalls(ast, parsedLogics, filename, ({ path, parsedLogic }) => {
        const stmt = path.node
        const arg = stmt.arguments[0]

        if (t.ObjectExpression.check(arg)) {
            const propertiesMap = Object.fromEntries(
                arg.properties
                    .map((p) => [t.ObjectProperty.check(p) && t.Identifier.check(p.key) ? p.key.name : null, p])
                    .filter(([key]) => key !== null),
            )
            const propertyKeys = Object.keys(propertiesMap)

            const newEntries = []

            for (const [key, importFromOrArray] of Object.entries(supportedProperties)) {
                if (propertiesMap[key] && t.ObjectProperty.check(propertiesMap[key])) {
                    const importFrom = Array.isArray(importFromOrArray) ? importFromOrArray[0] : importFromOrArray
                    const renameTo =
                        Array.isArray(importFromOrArray) && importFromOrArray.length > 1 ? importFromOrArray[1] : key

                    newEntries.push(b.callExpression(b.identifier(renameTo), [propertiesMap[key].value]))
                    neededImports[importFrom] ??= []
                    if (!neededImports[importFrom].find(([l]) => l === renameTo)) {
                        neededImports[importFrom].push([renameTo, renameTo])
                    }
                }
            }

            const unsupported = propertyKeys.filter((p) => !supportedProperties[p])
            if (unsupported.length > 0) {
                console.warn(
                    `❗ Logic "${parsedLogic.logicName}", converted unsupported keys (${unsupported.join(
                        ', ',
                    )}) to builders without imports`,
                )
            }
            for (const key of unsupported) {
                if (propertiesMap[key] && t.ObjectProperty.check(propertiesMap[key])) {
                    newEntries.push(b.callExpression(b.identifier(key), [propertiesMap[key].value]))
                }
            }

            stmt.arguments[0] = b.arrayExpression(newEntries)
        }
    })

    for (const [importFrom, neededImportsFromThisKey] of Object.entries(neededImports)) {
        for (const [importName, localName] of neededImportsFromThisKey) {
            if (!imports[importFrom] || !imports[importFrom].find(([i, l]) => i === importName && l === localName)) {
                assureImport(ast, importFrom, importName, localName, !!imports[importFrom])
                imports[importFrom] ??= []
                imports[importFrom].push([importName, localName])
            }
        }
    }

    const newText = runThroughPrettier(print(ast).code, filename)
    fs.writeFileSync(filename, newText)

    log(`🔥 Converted to builders: ${osPath.relative(process.cwd(), filename)}`)
}
