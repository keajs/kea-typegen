#!/usr/bin/env node

import * as ts from 'typescript'
import * as yargs from 'yargs'
import * as path from 'path'
import { visitProgram } from '../visit/visit'
import { printToFiles } from '../print/print'

const parser = yargs
    .usage('Use one of:\n - kea-typegen -f logic.ts\n - kea-typegen -c tsconfig.json')
    .option('f', { alias: 'file', describe: 'Logic file', type: 'string' })
    .option('c', { alias: 'config', describe: 'Path to tsconfig.json', type: 'string' })
    .option('write', { describe: 'Write logic.type.ts files', type: 'string' })
    .option('w', { alias: 'watch', describe: 'Watch for changes', type: 'string' })

const options = parser.argv

let program

const write = typeof options.write !== 'undefined'
const watch = typeof options.watch !== 'undefined'

if (options.file) {
    console.log(`Loading file: ${options.file}`)
    program = ts.createProgram([options.file as string], {
        target: ts.ScriptTarget.ES5,
        module: ts.ModuleKind.CommonJS,
    })
} else {
    const configFileName = (options.config || ts.findConfigFile('./', ts.sys.fileExists, 'tsconfig.json')) as string

    if (configFileName) {
        console.log(`Using Config: ${configFileName}`)
        const configFile = ts.readJsonConfigFile(configFileName as string, ts.sys.readFile)
        const rootFolder = path.resolve(configFileName.replace(/tsconfig\.json$/, ''))
        const compilerOptions = ts.parseJsonSourceFileConfigFileContent(configFile, ts.sys, rootFolder)

        if (watch) {
            const createProgram = ts.createSemanticDiagnosticsBuilderProgram

            const host = ts.createWatchCompilerHost(
                configFileName,
                compilerOptions.options,
                ts.sys,
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

                goThroughAllTheFiles(program, write)
            }

            ts.createWatchProgram(host)
        } else {
            const host = ts.createCompilerHost(compilerOptions.options)
            program = ts.createProgram(compilerOptions.fileNames, compilerOptions.options, host)
        }
    }
}

function goThroughAllTheFiles(program, write) {
    const parsedLogics = visitProgram(program, true)
    console.log(`## ${parsedLogics.length} logic${parsedLogics.length === 1 ? '' : 's'} found!`)
    if (write) {
        printToFiles(parsedLogics, true)
    } else {
        console.log('Run with --write to write logic.type.ts files')
    }
}

if (program) {
    if (!watch && !options.file) {
        goThroughAllTheFiles(program, write)
    }
} else {
    parser.showHelp()
}
