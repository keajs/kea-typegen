const fs = require('fs')
const Module = require('module')
const path = require('path')

require('ts-node/register/transpile-only')

const repoRoot = path.resolve(__dirname, '..', '..')
const projectDir = fs.mkdtempSync(path.join(repoRoot, 'tmp-write-smoke-'))
const tsconfigPath = path.join(projectDir, 'tsconfig.json')
const logicFilePath = path.join(projectDir, 'src', 'logic.ts')

const noop = () => {}

let createProgramCalls = 0
let reusedOldProgram = false
let finished = false
const exitSignalKey = '__writeModeSmokeExit'

const originalTs = require('typescript')
const instrumentedTs = new Proxy(originalTs, {
    get(target, property, receiver) {
        if (property === 'createProgram') {
            return function (...args) {
                createProgramCalls += 1
                reusedOldProgram = reusedOldProgram || !!args[3]
                return Reflect.get(target, property, receiver).apply(this, args)
            }
        }

        return Reflect.get(target, property, receiver)
    },
})

const originalModuleLoad = Module._load
Module._load = function (request, parent, isMain) {
    if (request === 'typescript') {
        return instrumentedTs
    }

    return originalModuleLoad.apply(this, arguments)
}

function cleanup() {
    try {
        fs.rmSync(projectDir, { recursive: true, force: true })
    } catch {}
}

function finish(code, error) {
    if (finished) {
        return
    }
    finished = true

    cleanup()

    if (error) {
        console.error(error)
    }

    fs.writeFileSync(
        process.stdout.fd,
        JSON.stringify({
            createProgramCalls,
            reusedOldProgram,
        }) + '\n',
    )

    process.exitCode = code
}

const originalProcessExit = process.exit
process.exit = (code) => {
    throw { [exitSignalKey]: true, code: code ?? 0 }
}

process.on('uncaughtException', (error) => {
    finish(1, error)
})

process.on('unhandledRejection', (error) => {
    finish(1, error)
})

fs.mkdirSync(path.dirname(logicFilePath), { recursive: true })
fs.writeFileSync(
    tsconfigPath,
    JSON.stringify(
        {
            compilerOptions: {
                target: 'ES2020',
                module: 'commonjs',
                moduleResolution: 'node',
                esModuleInterop: true,
                skipLibCheck: true,
                strict: false,
            },
            include: ['src/**/*'],
        },
        null,
        2,
    ),
)

fs.writeFileSync(
    logicFilePath,
    [
        "import { kea } from 'kea'",
        '',
        'export const logic = kea({',
        '    actions: () => ({',
        '        setValue: (value: string) => ({ value }),',
        '    }),',
        '    reducers: () => ({',
        "        value: ['', { setValue: (_, payload) => payload.value }],",
        '    }),',
        '})',
        '',
    ].join('\n'),
)

const { runTypeGen } = require(path.join(repoRoot, 'src/typegen'))

Promise.resolve(
    runTypeGen({
        tsConfigPath: tsconfigPath,
        rootPath: projectDir,
        typesPath: projectDir,
        write: true,
        watch: false,
        log: noop,
    }),
)
    .then(() => {
        finish(0)
    })
    .catch((error) => {
        if (error && error[exitSignalKey]) {
            finish(error.code)
        } else {
            finish(1, error)
        }
    })
    .finally(() => {
        Module._load = originalModuleLoad
        process.exit = originalProcessExit
    })
