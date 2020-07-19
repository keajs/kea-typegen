#!/usr/bin/env node

import * as ts from 'typescript'
import * as yargs from 'yargs'
import * as path from 'path'
import { visitProgram } from '../visit/visit'
import { printToFiles } from '../print/print'
import { AppOptions } from '../types'

// NB! Sync this with the AppOptions type
const parser = yargs
    .usage('Use one of:\n - kea-typegen   # no arguments, searches for tsconfig.json \n - kea-typegen -c tsconfig.json\n - kea-typegen -f logic.ts')
    .option('file', { alias: 'f', describe: 'Logic file to evaluate (without tsconfig.json)', type: 'string' })
    .option('config', { alias: 'c', describe: 'Path to tsconfig.json', type: 'string' })
    .option('quiet', { alias: 'q', describe: 'Produce no output', type: 'boolean' })
    .option('verbose', { describe: 'Produce a bit more output', type: 'boolean' })
    .option('write', { alias: 'W', describe: 'Write logic.type.ts files', type: 'boolean' })
    .option('path', {
        alias: 'p',
        describe: 'Start logic path autogeneration from here. E.g: ./frontend/src',
        type: 'string',
    })
    .option('watch', {
        alias: 'w',
        describe: 'Watch for changes (NB! Only works with tsconfig.json files!)',
        type: 'boolean',
    })
// Read the NB above

const parsedOptions = parser.argv

let program

const appOptions = {
    logicStartPath: parsedOptions.path,
    tsConfigPath: parsedOptions.cofig,
    sourceFilePath: parsedOptions.file,
    write: parsedOptions.write,
    watch: parsedOptions.watch,
    quiet: parsedOptions.quiet,
    verbose: parsedOptions.verbose,
    log: parsedOptions.quiet ? () => null : console.log.bind(console)
} as AppOptions

const { log } = appOptions

if (appOptions.sourceFilePath) {
    log(`Loading file: ${appOptions.sourceFilePath}`)
    program = ts.createProgram([appOptions.sourceFilePath as string], {
        target: ts.ScriptTarget.ES5,
        module: ts.ModuleKind.CommonJS,
        noEmit: true,
    })
} else {
    const configFileName = (appOptions.tsConfigPath || ts.findConfigFile('./', ts.sys.fileExists, 'tsconfig.json')) as string

    if (configFileName) {
        log(`Using Config: ${configFileName}`)
        log('')

        const configFile = ts.readJsonConfigFile(configFileName as string, ts.sys.readFile)
        const rootFolder = path.resolve(configFileName.replace(/tsconfig\.json$/, ''))
        const compilerOptions = ts.parseJsonSourceFileConfigFileContent(configFile, ts.sys, rootFolder)

        if (appOptions.watch) {
            const createProgram = ts.createEmitAndSemanticDiagnosticsBuilderProgram

            const host = ts.createWatchCompilerHost(
                configFileName,
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
                console.error(
                    'Error',
                    diagnostic.code,
                    ':',
                    ts.flattenDiagnosticMessageText(diagnostic.messageText, formatHost.getNewLine()),
                )
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
            const host = ts.createCompilerHost(compilerOptions.options)
            program = ts.createProgram(compilerOptions.fileNames, compilerOptions.options, host)
        }
    }
}

function goThroughAllTheFiles(program, appOptions) {
    const parsedLogics = visitProgram(program, appOptions)
    if (appOptions?.verbose) {
        log('')
    }
    log(`## ${parsedLogics.length} logic${parsedLogics.length === 1 ? '' : 's'} found!`)
    log('')

    printToFiles(appOptions, parsedLogics)
}

if (program) {
    if (!appOptions.watch && !appOptions.sourceFilePath) {
        goThroughAllTheFiles(program, appOptions)
    }
} else {
    parser.showHelp()
}
