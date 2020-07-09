import * as ts from 'typescript'
import * as yargs from 'yargs'
import * as path from 'path'
import { visitProgram } from '../visit/visit'
import { printToFiles } from '../print/print'

const parser = yargs
    .usage('Use one of:\n - kea-typegen -f logic.ts\n - kea-typegen -c tsconfig.json')
    .option('f', { alias: 'file', describe: 'Logic file', type: 'string' })
    .option('c', { alias: 'config', describe: 'Path to tsconfig.json', type: 'string' })
    .option('w', { alias: 'write', describe: 'Write logic.type.ts files', type: 'string' })

const options = parser.argv

let program

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
        const host = ts.createCompilerHost(compilerOptions.options)

        program = ts.createProgram(compilerOptions.fileNames, compilerOptions.options, host)
    }
}

if (program) {
    const parsedLogics = visitProgram(program, true)
    console.log(`Found ${parsedLogics.length} logic${parsedLogics.length === 1 ? '' : 's'}!`)
    if (typeof options.write !== 'undefined') {
        printToFiles(parsedLogics, true)
    } else {
        console.log('Run with --write to write logic.type.ts files')
    }
} else {
    parser.showHelp()
}
