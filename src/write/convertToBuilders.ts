import { AppOptions, ParsedLogic } from '../types'
import * as ts from 'typescript'
import { parse, print, visit } from 'recast'
import { runThroughPrettier } from '../print/print'
import * as fs from 'fs'
import * as osPath from 'path'
import { t, b, visitAllKeaCalls, assureImport } from './utils'
import { ExpressionKind } from 'ast-types/gen/kinds'

const supportedProperties = {
    props: 'kea',
    key: 'kea',
    path: 'kea',
    connect: 'kea',
    actions: 'kea',
    defaults: 'kea',
    loaders: 'kea-loaders',
    windowValues: 'kea-windowvalues',
    reducers: 'kea',
    selectors: 'kea',
    sharedListeners: 'kea',
    listeners: 'kea',
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

    const ast = parse(rawCode, {
        parser: require('recast/parsers/typescript'),
    })

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
                imports[importFrom].push([specifier.imported.name, specifier.local.name])
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

            const unsupported = propertyKeys.filter((p) => !supportedProperties[p])
            if (unsupported.length > 0) {
                console.error(
                    `Can not convert logic ${
                        parsedLogic.logicName
                    } to Kea 3.0 builders due to unsupported input keys: ${unsupported.join(', ')}. Skipping.`,
                )
                return
            }

            const newEntries = []

            for (const [key, importFrom] of Object.entries(supportedProperties)) {
                if (propertiesMap[key] && t.ObjectProperty.check(propertiesMap[key])) {
                    newEntries.push(b.callExpression(b.identifier(key), [propertiesMap[key].value]))
                    neededImports[importFrom] ??= []
                    if (!neededImports[importFrom].find(([l]) => l === key)) {
                        neededImports[importFrom].push([key, key])
                    }
                }
            }

            stmt.arguments[0] = b.arrayExpression(newEntries)
        }
    })

    for (const [importFrom, neededImportsFromThisKey] of Object.entries(neededImports)) {
        for (const [importName, localName] of neededImportsFromThisKey) {
            if (!imports[importFrom] || !imports[importFrom].find(([i, l]) => i === importName && l === localName)) {
                assureImport(ast, importFrom, importName, localName, !!imports[importFrom])
            }
        }
    }

    const newText = runThroughPrettier(print(ast).code, filename)
    fs.writeFileSync(filename, newText)

    log(`🔥 Converted to builders: ${osPath.relative(process.cwd(), filename)}`)
}
