#!/usr/bin/env node

import * as ts from 'typescript'
import * as yargs from 'yargs'
import * as path from 'path'
import * as fs from 'fs'
import { AppOptions } from '../types'
import { runTypeGen } from '../typegen'

yargs
    .command(
        'check',
        '- check what should be done',
        (yargs) => {},
        (argv) => {
            runTypeGen({ ...includeKeaConfig(parsedToAppOptions(argv)), write: false, watch: false })
        },
    )
    .command(
        'write',
        '- write logicType.ts files',
        (yargs) => {},
        (argv) => {
            runTypeGen({ ...includeKeaConfig(parsedToAppOptions(argv)), write: true, watch: false })
        },
    )
    .command(
        'watch',
        '- watch for changes and write logicType.ts files',
        (yargs) => {},
        (argv) => {
            runTypeGen({ ...includeKeaConfig(parsedToAppOptions(argv)), write: true, watch: true })
        },
    )
    .option('config', { alias: 'c', describe: 'Path to tsconfig.json (otherwise auto-detected)', type: 'string' })
    .option('file', { alias: 'f', describe: "Single file to evaluate (can't be used with --config)", type: 'string' })
    .option('root', {
        alias: 'r',
        describe: 'Root for logic paths. E.g: ./frontend/src',
        type: 'string',
    })
    .option('types', {
        alias: 't',
        describe: 'Folder to write logicType.ts files to.\nWrites alongside logic.ts files by default.',
        type: 'string',
    })
    .option('quiet', { alias: 'q', describe: 'Write nothing to stdout', type: 'boolean' })
    .option('no-import', { describe: 'Do not automatically import generated types in logic files', type: 'boolean' })
    .option('write-paths', { describe: 'Write paths into logic files that have none', type: 'boolean' })
    .option('import-global-types', {
        describe: 'Add import statements in logicType.ts files for global types (e.g. @types/node)',
        type: 'boolean',
    })
    .option('ignore-import-paths', {
        describe: 'List of paths we will never import from inside logicType.ts files',
        type: 'array',
    })
    .option('verbose', { describe: 'Slightly more verbose output log', type: 'boolean' })
    .demandCommand()
    .help()
    .wrap(80).argv

function parsedToAppOptions(parsedOptions) {
    const appOptions = {
        rootPath: parsedOptions.root,
        typesPath: parsedOptions.types,
        tsConfigPath: parsedOptions.config,
        sourceFilePath: parsedOptions.file,
        quiet: parsedOptions.quiet,
        verbose: parsedOptions.verbose,
        noImport: parsedOptions.noImport,
        writePaths: parsedOptions.writePaths,
        importGlobalTypes: parsedOptions.importGlobalTypes,
        ignoreImportPaths: parsedOptions.ignoreImportPaths,
        log: parsedOptions.quiet ? () => null : console.log.bind(console),
    } as AppOptions

    return appOptions
}

function findKeaConfig(searchPath): string {
    return ts.findConfigFile(searchPath, ts.sys.fileExists, '.kearc')
}

function includeKeaConfig(appOptions: AppOptions): AppOptions {
    const configFilePath = findKeaConfig(appOptions.rootPath)
    const newOptions = { ...appOptions } as AppOptions

    let rawData, keaConfig

    // first, set all CLI argument paths relative from process.cwd()
    for (const key of Object.keys(appOptions)) {
        if (key.endsWith('Path') && appOptions[key]) {
            newOptions[key] = path.resolve(process.cwd(), appOptions[key])
        }
    }

    // has .kearc
    if (configFilePath) {
        const configDirPath = path.dirname(configFilePath)
        try {
            rawData = fs.readFileSync(configFilePath)
        } catch (e) {
            console.error(`Error reading Kea config file: ${configFilePath}`)
            process.exit(1)
        }
        try {
            keaConfig = JSON.parse(rawData)
        } catch (e) {
            console.error(`Error parsing Kea config JSON: ${configFilePath}`)
            process.exit(1)
        }

        Object.keys(keaConfig)
            .filter((key) => typeof newOptions[key] === 'undefined')
            .forEach((key) => {
                // set all paths in .kearc to be relative from where the file is located
                if (key.endsWith('Path')) {
                    newOptions[key] = path.resolve(process.cwd(), configDirPath, keaConfig[key])
                } else if (key.endsWith('Paths')) {
                    if (!Array.isArray(keaConfig[key])) {
                        console.error(`Config "${key}" in ".kearc" is not an array`)
                        process.exit(1)
                    }
                    newOptions[key] = keaConfig[key].map((value) => path.resolve(process.cwd(), configDirPath, value))
                } else {
                    newOptions[key] = keaConfig[key]
                }
            })
    }

    if (!newOptions.rootPath) {
        newOptions.rootPath = newOptions.tsConfigPath ? path.dirname(newOptions.tsConfigPath) : process.cwd()
    }

    if (!newOptions.typesPath) {
        newOptions.typesPath = newOptions.rootPath
    }

    if (!newOptions.tsConfigPath) {
        newOptions.tsConfigPath =
            ts.findConfigFile(newOptions.rootPath, ts.sys.fileExists, 'tsconfig.json') ||
            ts.findConfigFile('./', ts.sys.fileExists, 'tsconfig.json')
    }

    if (!newOptions.packageJsonPath) {
        newOptions.packageJsonPath =
            ts.findConfigFile(newOptions.rootPath, ts.sys.fileExists, 'package.json') ||
            ts.findConfigFile('./', ts.sys.fileExists, 'package.json')
    }

    return newOptions
}
