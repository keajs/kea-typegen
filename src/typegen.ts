import * as ts from 'typescript'
import * as path from 'path'
import { visitProgram } from './visit/visit'
import { printToFiles } from './print/print'
import { AppOptions } from './types'
import { Program } from 'typescript'
import { version } from '../package.json'

export function runTypeGen(appOptions: AppOptions) {
    let program: Program
    let resetProgram: () => void

    const { log } = appOptions
    log(`🦜 Kea-TypeGen v${version}`)

    if (appOptions.sourceFilePath) {
        log(`❇️ Loading file: ${appOptions.sourceFilePath}`)
        resetProgram = () => {
            program = ts.createProgram([appOptions.sourceFilePath], {
                target: ts.ScriptTarget.ES5,
                module: ts.ModuleKind.CommonJS,
                noEmit: true,
            })
        }
        resetProgram()
    } else if (appOptions.tsConfigPath) {
        log(`🥚 TypeScript Config: ${appOptions.tsConfigPath}`)

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

            console.info(`🥚 TypeScript Compiler API v${ts.version}`)

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
                const codes = {
                    6031: `👀 Starting TypeScript watch mode`,
                    6032: `🔄 Reloading...`,
                }
                console.info(codes[diagnostic.code] || `🥚 ${ts.formatDiagnostic(diagnostic, formatHost).trim()}`)
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
        log(`⛔ No tsconfig.json found! No source file specified.`)
    }

    function goThroughAllTheFiles(
        program,
        appOptions,
    ): { filesToWrite: number; writtenFiles: number; filesToModify: number } {
        const parsedLogics = visitProgram(program, appOptions)
        if (appOptions.verbose) {
            log(`🗒️ ${parsedLogics.length} logic${parsedLogics.length === 1 ? '' : 's'} found!`)
        }

        const response = printToFiles(program, appOptions, parsedLogics)

        // running "kea-typegen check" and would write files?
        // exit with 1
        if (!appOptions.write && !appOptions.watch && (response.filesToWrite > 0 || response.filesToModify > 0)) {
            process.exit(1)
        }

        return response
    }

    if (program && !appOptions.watch && !appOptions.sourceFilePath) {
        if (appOptions.write) {
            let round = 0
            while ((round += 1)) {
                const { writtenFiles, filesToModify } = goThroughAllTheFiles(program, appOptions)

                if (writtenFiles === 0 && filesToModify === 0) {
                    log(`👋 Finished writing files! Exiting.`)
                    process.exit(0)
                }

                if (round > 50) {
                    log(`🔁 We seem to be stuck in a loop (ran %{round} times)! Exiting!`)
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
