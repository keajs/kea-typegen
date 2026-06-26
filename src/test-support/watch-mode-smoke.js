const fs = require('fs')
const os = require('os')
const path = require('path')

require('ts-node/register/transpile-only')

const repoRoot = path.resolve(__dirname, '..', '..')
const projectDir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-watch-smoke-'))
const tsconfigPath = path.join(projectDir, 'tsconfig.json')
const keaDtsPath = path.join(projectDir, 'node_modules', 'kea', 'index.d.ts')
const fileCount = 200
const logicPaths = []

const noop = () => {}
console.info = noop

let active = 0
let maxActive = 0
let started = 0
let completed = 0
let settleTimer
let timeoutTimer
let finished = false
let editTriggered = false
let incrementalAfterEdit = false
const parsedLogicCountsAfterEdit = []
const sourceFilePathsAfterEdit = []

function cleanup() {
    try {
        fs.rmSync(projectDir, { recursive: true, force: true })
    } catch {}
}

function finish(code) {
    if (finished) {
        return
    }
    finished = true
    clearTimeout(settleTimer)
    clearTimeout(timeoutTimer)
    cleanup()

    fs.writeFileSync(
        process.stdout.fd,
        JSON.stringify({
            started,
            completed,
            active,
            maxActive,
            incrementalAfterEdit,
            parsedLogicCountsAfterEdit,
            sourceFilePathsAfterEdit,
        }) + '\n',
    )
    process.exit(code)
}

function scheduleFinishIfSettled() {
    if (active !== 0) {
        return
    }

    clearTimeout(settleTimer)
    settleTimer = setTimeout(() => {
        if (!editTriggered && started > 1) {
            editTriggered = true
            fs.appendFileSync(logicPaths[0], '\n// trigger incremental watch pass\n')
            return
        }

        if (editTriggered && incrementalAfterEdit) {
            finish(0)
        }
    }, 300)
}

process.on('uncaughtException', (error) => {
    console.error(error)
    finish(1)
})

process.on('unhandledRejection', (error) => {
    console.error(error)
    finish(1)
})

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

fs.mkdirSync(path.dirname(keaDtsPath), { recursive: true })
fs.writeFileSync(keaDtsPath, 'export function kea<T = any>(input: any): T\n')

for (let i = 0; i < fileCount; i++) {
    const dir = path.join(projectDir, 'src', `group${String(i % 10).padStart(2, '0')}`)
    const filePath = path.join(dir, `logic${i}.ts`)
    logicPaths.push(filePath)

    fs.mkdirSync(dir, { recursive: true })
    fs.writeFileSync(
        filePath,
        [
            "import { kea } from 'kea'",
            '',
            `export const logic${i} = kea({`,
            '    actions: () => ({',
            '        setValue: (value: string) => ({ value }),',
            '    }),',
            '    reducers: () => ({',
            "        value: ['' as string, { setValue: (_, payload) => payload.value }],",
            '    }),',
            '})',
            '',
        ].join('\n'),
    )
}

const printModule = require(path.join(repoRoot, 'src/print/print'))
const originalPrintToFiles = printModule.printToFiles

printModule.printToFiles = async function (...args) {
    const appOptions = args[1]
    const parsedLogics = args[2]

    active += 1
    started += 1
    maxActive = Math.max(maxActive, active)
    clearTimeout(settleTimer)

    if (editTriggered) {
        parsedLogicCountsAfterEdit.push(parsedLogics.length)
        sourceFilePathsAfterEdit.push(appOptions.sourceFilePath || null)
        incrementalAfterEdit = incrementalAfterEdit || (parsedLogics.length === 1 && !!appOptions.sourceFilePath)
    }

    await new Promise((resolve) => setTimeout(resolve, 25))

    try {
        return await originalPrintToFiles.apply(this, args)
    } finally {
        await new Promise((resolve) => setTimeout(resolve, 25))
        active -= 1
        completed += 1
        scheduleFinishIfSettled()
    }
}

const { runTypeGen } = require(path.join(repoRoot, 'src/typegen'))

timeoutTimer = setTimeout(() => finish(1), 20000)

runTypeGen({
    tsConfigPath: tsconfigPath,
    rootPath: projectDir,
    typesPath: projectDir,
    write: true,
    watch: true,
    log: noop,
})
