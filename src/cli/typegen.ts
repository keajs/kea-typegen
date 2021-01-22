#!/usr/bin/env node

import * as ts from 'typescript'
import * as yargs from 'yargs'
import * as path from 'path'
import * as fs from 'fs'
import { visitProgram } from '../visit/visit'
import { printToFiles } from '../print/print'
import { AppOptions } from '../types'
import { Program } from 'typescript'

const parser = yargs
    .command(
        'check',
        '- check what should be done',
        (yargs) => {},
        (argv) => {
            runCLI({ ...includeKeaConfig(parsedToAppOptions(argv)), write: false, watch: false })
        },
    )
    .command(
        'write',
        '- write logicType.ts files',
        (yargs) => {},
        (argv) => {
            runCLI({ ...includeKeaConfig(parsedToAppOptions(argv)), write: true, watch: false })
        },
    )
    .command(
        'watch',
        '- watch for changes and write logicType.ts files',
        (yargs) => {},
        (argv) => {
            runCLI({ ...includeKeaConfig(parsedToAppOptions(argv)), write: true, watch: true })
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
        log: parsedOptions.quiet ? () => null : console.log.bind(console),
    } as AppOptions

    return appOptions
}

function findKeaConfig(): string {
    return ts.findConfigFile('./', ts.sys.fileExists, '.kearc')
}

function includeKeaConfig(appOptions: AppOptions): AppOptions {
    const configFilePath = findKeaConfig()
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

        Object.keys(newOptions)
            .filter((key) => keaConfig[key])
            .forEach((key) => {
                // set all paths in .kearc to be relative from where the file is located
                if (key.endsWith('Path')) {
                    newOptions[key] = path.resolve(process.cwd(), configDirPath, keaConfig[key])
                } else {
                    newOptions[key] = keaConfig[key]
                }
            })
    }

    if (!newOptions.tsConfigPath) {
        newOptions.tsConfigPath = ts.findConfigFile('./', ts.sys.fileExists, 'tsconfig.json')
    }

    if (!newOptions.rootPath) {
        newOptions.rootPath = newOptions.tsConfigPath ? path.dirname(newOptions.tsConfigPath) : process.cwd()
    }

    if (!newOptions.typesPath) {
        newOptions.typesPath = newOptions.rootPath
    }

    return newOptions
}

function runCLI(appOptions: AppOptions) {
    let program: Program
    let resetProgram: () => void

    const { log } = appOptions

    if (appOptions.sourceFilePath) {
        log(`Loading file: ${appOptions.sourceFilePath}`)
        resetProgram = () => {
            program = ts.createProgram([appOptions.sourceFilePath], {
                target: ts.ScriptTarget.ES5,
                module: ts.ModuleKind.CommonJS,
                noEmit: true,
            })
        }
        resetProgram()
    } else if (appOptions.tsConfigPath) {
        log(`Using TypeScript Config: ${appOptions.tsConfigPath}`)
        log('')

        const configFile = ts.readJsonConfigFile(appOptions.tsConfigPath, ts.sys.readFile)
        const rootFolder = path.dirname(appOptions.tsConfigPath)
        const compilerOptions = ts.parseJsonSourceFileConfigFileContent(configFile, ts.sys, rootFolder)

        if (appOptions.watch) {
            const createProgram = ts.createEmitAndSemanticDiagnosticsBuilderProgram

            const host = ts.createWatchCompilerHost(
                appOptions.tsConfigPath,
                compilerOptions.options,
                {
                    ...ts.sys,
                    writeFile(path: string, data: string, writeByteOrderMark?: boolean) {
                        // skip emit
                        // https://github.com/microsoft/TypeScript/issues/32385
                        // https://github.com/microsoft/TypeScript/issues/36917
                        return null
                    },
                },
                createProgram,
                reportDiagnostic,
                reportWatchStatusChanged,
            )

            const formatHost: ts.FormatDiagnosticsHost = {
                getCanonicalFileName: (path) => path,
                getCurrentDirectory: ts.sys.getCurrentDirectory,
                getNewLine: () => ts.sys.newLine,
            }

            function reportDiagnostic(diagnostic: ts.Diagnostic) {
                if (appOptions.verbose) {
                    console.error(
                        'Error',
                        diagnostic.code,
                        ':',
                        ts.flattenDiagnosticMessageText(diagnostic.messageText, formatHost.getNewLine()),
                    )
                }
            }

            function reportWatchStatusChanged(diagnostic: ts.Diagnostic) {
                console.info(ts.formatDiagnostic(diagnostic, formatHost))
            }

            const origCreateProgram = host.createProgram
            host.createProgram = (rootNames: ReadonlyArray<string>, options, host, oldProgram) => {
                return origCreateProgram(rootNames, options, host, oldProgram)
            }
            const origPostProgramCreate = host.afterProgramCreate

            host.afterProgramCreate = (prog) => {
                program = prog.getProgram()
                origPostProgramCreate!(prog)

                goThroughAllTheFiles(program, appOptions)
            }

            ts.createWatchProgram(host)
        } else {
            resetProgram = () => {
                const host = ts.createCompilerHost(compilerOptions.options)
                program = ts.createProgram(compilerOptions.fileNames, compilerOptions.options, host)
            }
            resetProgram()
        }
    } else {
        log(`No tsconfig.json found! No source file specified.`)
    }

    function goThroughAllTheFiles(program, appOptions): { filesToWrite: number; writtenFiles: number } {
        const parsedLogics = visitProgram(program, appOptions)
        if (appOptions?.verbose) {
            log('')
        }
        log(`## ${parsedLogics.length} logic${parsedLogics.length === 1 ? '' : 's'} found!`)
        log('')

        const response = printToFiles(appOptions, parsedLogics)

        // running "kea-typegen check" and would write files?
        // exit with 1
        if (!appOptions.write && !appOptions.watch && response.filesToWrite > 0) {
            process.exit(1)
        }

        return response
    }

    if (program && !appOptions.watch && !appOptions.sourceFilePath) {
        if (appOptions.write) {
            let round = 0
            while ((round += 1)) {
                const { writtenFiles } = goThroughAllTheFiles(program, appOptions)

                if (writtenFiles === 0) {
                    log(`Finished writing files! Exiting.`)
                    process.exit(0)
                }

                if (round > 50) {
                    log(`We seem to be stuck in a loop (ran %{round} times)! Exiting!`)
                    process.exit(1)
                }

                resetProgram()
            }
        } else {
            // check them once
            goThroughAllTheFiles(program, appOptions)
        }
    }
}
