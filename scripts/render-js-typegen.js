#!/usr/bin/env node

const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

function printHelp() {
    console.log(`Usage: node ./scripts/render-js-typegen.js --config <tsconfig> --file <logic file> [options]

Options:
      --config <path>      Path to tsconfig.json
      --file <path>        Source logic file to render
      --root <path>        Root for logic paths
      --types <path>       Folder to write generated Type files
      --package-json <path> Path to package.json
      --rounds <count>     Render rounds for generated dependency surfaces (default: 2)
      --add-ts-nocheck     Add @ts-nocheck to generated output
      --import-global-types Import global types instead of omitting configured compiler types
  -h, --help               Show this help
`)
}

function parseArgs(argv) {
    const options = {
        config: '',
        file: '',
        root: '',
        types: '',
        packageJson: '',
        rounds: 2,
        addTsNocheck: false,
        importGlobalTypes: false,
    }

    for (let index = 0; index < argv.length; index++) {
        const arg = argv[index]
        if ((arg === '--config' || arg === '-c') && argv[index + 1]) {
            options.config = path.resolve(argv[++index])
        } else if ((arg === '--file' || arg === '-f') && argv[index + 1]) {
            options.file = path.resolve(argv[++index])
        } else if ((arg === '--root' || arg === '-r') && argv[index + 1]) {
            options.root = path.resolve(argv[++index])
        } else if ((arg === '--types' || arg === '-t') && argv[index + 1]) {
            options.types = path.resolve(argv[++index])
        } else if (arg === '--package-json' && argv[index + 1]) {
            options.packageJson = path.resolve(argv[++index])
        } else if (arg === '--rounds' && argv[index + 1]) {
            options.rounds = Number(argv[++index])
        } else if (arg === '--add-ts-nocheck') {
            options.addTsNocheck = true
        } else if (arg === '--import-global-types') {
            options.importGlobalTypes = true
        } else if (arg === '--help' || arg === '-h') {
            printHelp()
            process.exit(0)
        } else {
            throw new Error(`Unknown argument: ${arg}`)
        }
    }

    if (!options.config) {
        throw new Error('Missing --config')
    }
    if (!options.file) {
        throw new Error('Missing --file')
    }
    if (!options.root) {
        options.root = path.dirname(options.config)
    }
    if (!options.types) {
        options.types = options.root
    }
    if (!options.packageJson) {
        options.packageJson =
            findConfigUp(options.root, 'package.json') || findConfigUp(path.dirname(options.config), 'package.json')
    }
    if (!Number.isInteger(options.rounds) || options.rounds < 1) {
        throw new Error(`Invalid --rounds value: ${options.rounds}`)
    }

    return options
}

function findConfigUp(startPath, fileName) {
    let current = path.resolve(startPath)
    while (true) {
        const candidate = path.join(current, fileName)
        if (fs.existsSync(candidate)) {
            return candidate
        }
        const parent = path.dirname(current)
        if (parent === current) {
            return ''
        }
        current = parent
    }
}

function loadImplementation(repoRoot) {
    const requestedMode = process.env.KEA_TYPEGEN_JS_MODE || 'auto'
    const distVisit = path.join(repoRoot, 'dist', 'src', 'visit', 'visit.js')
    const distPrint = path.join(repoRoot, 'dist', 'src', 'print', 'print.js')

    let selectedMode = requestedMode
    if (requestedMode === 'auto') {
        selectedMode = fs.existsSync(distVisit) && fs.existsSync(distPrint) ? 'dist' : 'source'
    }

    if (selectedMode === 'dist') {
        if (!fs.existsSync(distVisit) || !fs.existsSync(distPrint)) {
            throw new Error(
                'Compiled JS typegen modules not found; run bin/prepare-js or use KEA_TYPEGEN_JS_MODE=source',
            )
        }
        return {
            visitProgram: require(distVisit).visitProgram,
            printToFiles: require(distPrint).printToFiles,
        }
    }

    require(path.join(repoRoot, 'node_modules', 'ts-node', 'register'))
    return {
        visitProgram: require(path.join(repoRoot, 'src', 'visit', 'visit.ts')).visitProgram,
        printToFiles: require(path.join(repoRoot, 'src', 'print', 'print.ts')).printToFiles,
    }
}

function createProgramFromConfig(configPath, sourceFile) {
    const configFile = ts.readJsonConfigFile(configPath, ts.sys.readFile)
    const rootFolder = path.dirname(configPath)
    const compilerOptions = ts.parseJsonSourceFileConfigFileContent(configFile, ts.sys, rootFolder)
    return ts.createProgram([sourceFile], {
        ...compilerOptions.options,
        noEmit: true,
        noErrorTruncation: true,
    })
}

async function main() {
    const repoRoot = path.resolve(__dirname, '..')
    const options = parseArgs(process.argv.slice(2))
    const implementation = loadImplementation(repoRoot)
    const appOptions = {
        tsConfigPath: options.config,
        sourceFilePath: options.file,
        rootPath: options.root,
        typesPath: options.types,
        packageJsonPath: options.packageJson,
        addTsNocheck: options.addTsNocheck,
        importGlobalTypes: options.importGlobalTypes,
        debugPluginManifestPath: process.env.KEA_TYPEGEN_DEBUG_PLUGIN_MANIFEST,
        write: true,
        watch: false,
        noImport: true,
        writePaths: false,
        log: () => null,
    }

    for (let round = 0; round < options.rounds; round++) {
        const program = createProgramFromConfig(options.config, options.file)
        const parsedLogics = implementation.visitProgram(program, appOptions)
        const response = await implementation.printToFiles(program, appOptions, parsedLogics)
        if (response.writtenFiles === 0) {
            break
        }
    }
}

main().catch((error) => {
    console.error(error.message || String(error))
    process.exit(1)
})
