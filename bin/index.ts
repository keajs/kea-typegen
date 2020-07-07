// https://github.com/Microsoft/TypeScript/wiki/Using-the-Compiler-API#using-the-type-checker
import * as ts from 'typescript'
import { visitProgram } from '../src/visit/visit'
import * as yargs from 'yargs'

const parser = yargs
    .usage('Use one of:\n - kea-typegen -f logic.ts\n - kea-typegen -c tsconfig.json')
    .option('f', { alias: 'file', describe: 'Logic file', type: 'string' })
    .option('c', { alias: 'config', describe: 'Path to tsconfig.json', type: 'string' })

const options = parser.argv

let program

if (options.file) {
    program = ts.createProgram([options.file as string], {
        target: ts.ScriptTarget.ES5,
        module: ts.ModuleKind.CommonJS,
    })
} else {
    const configFileName = options.config || ts.findConfigFile('./', ts.sys.fileExists, 'tsconfig.json')

    if (configFileName) {
        console.log(`Using Config: ${configFileName}`)
        const configFile = ts.readConfigFile(configFileName as string, ts.sys.readFile)
        const compilerOptions = ts.parseJsonConfigFileContent(configFile.config, ts.sys, './')

        const host = ts.createCompilerHost(compilerOptions.options)
        program = ts.createProgram([], compilerOptions.options, host)
    }
}

if (program) {
    const parsedLogics = visitProgram(program)
} else {
    parser.showHelp()
}
