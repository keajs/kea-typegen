import * as ts from 'typescript'
import * as path from 'path'
import { visitProgram } from './visit/visit'
import { printToFiles } from './print/print'
import { AppOptions, ParsedLogic } from './types'
import { Program } from 'typescript'
import { openGoProgram } from './go'
import { openNativeProgram } from './native'
import { watchNativeProject } from './nativeWatch'
import { LogicProgram, OutputProgram, sourceFileNames } from './program'
import { version } from '../package.json'
import { restoreCachedTypes } from './cache'
import { createWatchChangeTracker, planWatchTypegenPass, WatchFileChange, WatchTypegenPlan } from './watch'

type ProgramSession = { close?(): void } & (
    | { program: LogicProgram }
    | { program: OutputProgram; parsed: ParsedLogic[] }
)

function projectCompiler(options: AppOptions): 'classic' | 'go' {
    const directory = path.dirname(path.resolve(options.tsConfigPath ?? options.sourceFilePath))
    try {
        const manifest = require.resolve('typescript/package.json', { paths: [directory] })
        return require(manifest).version.startsWith('7.') ? 'go' : 'classic'
    } catch (error) {
        if (error.code === 'MODULE_NOT_FOUND') return 'classic'
        throw error
    }
}

export async function runTypeGen(appOptions: AppOptions) {
    const { log } = appOptions
    log(`🦜 Kea-TypeGen v${version}`)
    if (!appOptions.tsConfigPath && !appOptions.sourceFilePath) throw new Error('Specify a tsconfig or source file.')

    appOptions = { ...appOptions, compiler: appOptions.compiler ?? projectCompiler(appOptions) }
    if (!['native', 'classic', 'go'].includes(appOptions.compiler)) {
        throw new Error(`Unknown typegen compiler: ${appOptions.compiler}`)
    }
    if (appOptions.compiler === 'native' || appOptions.compiler === 'go') {
        const open = appOptions.compiler === 'go' ? openGoProgram : openNativeProgram
        log(
            `🥚 Native TypeScript ${require('typescript-native/package.json').version}${appOptions.compiler === 'go' ? ' (Go typegen)' : ''}`,
        )
        const generate = (onProgramFiles?: (files: readonly string[]) => void) =>
            generateFiles(appOptions, () => open(appOptions), onProgramFiles)
        return appOptions.watch ? watchNativeProject(appOptions, generate) : generate()
    }

    const config = appOptions.tsConfigPath && loadTsConfig(appOptions.tsConfigPath, log)
    if (appOptions.watch && config && !appOptions.sourceFilePath) {
        return watchClassicProject(appOptions, config)
    }
    const compilerOptions = config?.options ?? {
        target: ts.ScriptTarget.ES5,
        module: ts.ModuleKind.CommonJS,
        noEmit: true,
        noErrorTruncation: true,
    }
    const files = appOptions.sourceFilePath ? [appOptions.sourceFilePath] : config.fileNames
    if (!appOptions.watch) {
        return generateFiles(appOptions, () => ({ program: ts.createProgram(files, compilerOptions) }))
    }
}

async function generateFiles(
    options: AppOptions,
    open: () => ProgramSession | Promise<ProgramSession>,
    onProgramFiles?: (files: readonly string[]) => void,
) {
    for (let round = 0; round < 51; round++) {
        const session = await open()
        try {
            onProgramFiles?.(sourceFileNames(session.program))
            const parsed = 'parsed' in session ? session.parsed : visitProgram(session.program, options)
            if (
                round === 0 &&
                options.write &&
                !options.sourceFilePath &&
                restoreCachedTypes(session.program, options, parsed)
            ) {
                continue
            }
            const { writtenFiles, filesToModify } = await printPass(session.program, options, parsed)
            if (!options.write || options.sourceFilePath || (!writtenFiles && !filesToModify)) {
                if (options.write && !options.watch) options.log('👋 Finished writing files! Exiting.')
                return
            }
        } finally {
            session.close?.()
        }
    }
    throw new Error('Type generation did not converge after 51 passes.')
}

function watchClassicProject(appOptions: AppOptions, compilerOptions: ts.ParsedCommandLine) {
    const { log } = appOptions
    // We don't emit JavaScript files in typegen watch mode, so the semantic-only
    // builder is enough and avoids extra emit-related work on every rebuild.
    const createProgram = ts.createSemanticDiagnosticsBuilderProgram
    const watchChangeTracker = createWatchChangeTracker(ts.sys)

    const host = ts.createWatchCompilerHost(
        appOptions.tsConfigPath,
        compilerOptions.options,
        {
            ...watchChangeTracker.system,
            writeFile(_path: string, _data: string, _writeByteOrderMark?: boolean) {
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
        if (appOptions.verbose || appOptions.showTsErrors) {
            log('💔 ' + ts.formatDiagnosticsWithColorAndContext([diagnostic], formatHost))
        }
    }

    function reportWatchStatusChanged(diagnostic: ts.Diagnostic) {
        const codes = {
            6031: `👀 Starting TypeScript watch mode`,
            6032: `🔄 Reloading...`,
        }
        console.info(codes[diagnostic.code] || `🥚 ${ts.formatDiagnostic(diagnostic, formatHost).trim()}`)
    }

    const origPostProgramCreate = host.afterProgramCreate
    let scheduledProgram: Program | undefined
    let scheduledChanges: WatchFileChange[] = []
    let runningTypegen = false

    const runScheduledTypegen = async () => {
        if (runningTypegen) {
            return
        }

        runningTypegen = true

        try {
            while (scheduledProgram) {
                const nextProgram = scheduledProgram
                const nextChanges = scheduledChanges
                scheduledProgram = undefined
                scheduledChanges = []

                try {
                    await goThroughWatchTypegenPlan(
                        nextProgram,
                        appOptions,
                        planWatchTypegenPass(nextProgram, nextChanges),
                    )
                } catch (error) {
                    console.error('⛔ Error running kea-typegen in watch mode')
                    console.error(error)
                }
            }
        } finally {
            runningTypegen = false

            if (scheduledProgram) {
                void runScheduledTypegen()
            }
        }
    }

    host.afterProgramCreate = (prog) => {
        const program = prog.getProgram()
        const changes = watchChangeTracker.consume()
        origPostProgramCreate?.(prog)
        scheduledProgram = program
        scheduledChanges.push(...changes)
        void runScheduledTypegen()
    }

    return ts.createWatchProgram(host)
}

async function goThroughAllTheFiles(
    program: LogicProgram,
    appOptions: AppOptions,
): Promise<{ filesToWrite: number; writtenFiles: number; filesToModify: number }> {
    return printPass(program, appOptions, visitProgram(program, appOptions))
}

async function printPass(program: OutputProgram, appOptions: AppOptions, parsedLogics: ParsedLogic[]) {
    if (appOptions.verbose) {
        appOptions.log(`🗒️ ${parsedLogics.length} logic${parsedLogics.length === 1 ? '' : 's'} found!`)
    }

    const response = await printToFiles(program, appOptions, parsedLogics)

    // running "kea-typegen check" and would write files?
    // exit with 1
    if (!appOptions.write && !appOptions.watch && (response.filesToWrite > 0 || response.filesToModify > 0)) {
        process.exitCode = 1
    }

    return response
}

async function goThroughWatchTypegenPlan(
    program,
    appOptions,
    plan: WatchTypegenPlan,
): Promise<{ filesToWrite: number; writtenFiles: number; filesToModify: number }> {
    if (plan.kind === 'skip') {
        if (appOptions.verbose) {
            appOptions.log(`⏭️ Skipping typegen watch pass: ${plan.reason}`)
        }
        return { filesToWrite: 0, writtenFiles: 0, filesToModify: 0 }
    }

    if (plan.kind === 'full') {
        if (appOptions.verbose) {
            appOptions.log(`🔎 Running full typegen watch pass: ${plan.reason}`)
        }
        return await goThroughAllTheFiles(program, appOptions)
    }

    if (appOptions.verbose) {
        appOptions.log(`🎯 Running incremental typegen watch pass: ${plan.reason}`)
    }

    const response = { filesToWrite: 0, writtenFiles: 0, filesToModify: 0 }

    for (const sourceFilePath of plan.sourceFilePaths) {
        const fileResponse = await goThroughAllTheFiles(program, {
            ...appOptions,
            delete: false,
            sourceFilePath,
        })

        response.filesToWrite += fileResponse.filesToWrite
        response.writtenFiles += fileResponse.writtenFiles
        response.filesToModify += fileResponse.filesToModify
    }

    return response
}

function loadTsConfig(tsConfigPath: string, log: AppOptions['log']): ts.ParsedCommandLine {
    log(`🥚 TypeScript Config: ${tsConfigPath}`)
    const configFile = ts.readJsonConfigFile(tsConfigPath, ts.sys.readFile)
    const rootFolder = path.dirname(tsConfigPath)
    return ts.parseJsonSourceFileConfigFileContent(configFile, ts.sys, rootFolder)
}

export function replaceProgram(createProgram: () => Program, setProgram: (program?: Program) => void): Program {
    setProgram(undefined)
    const nextProgram = createProgram()
    setProgram(nextProgram)
    return nextProgram
}
