const fs = require('fs')
const path = require('path')

require('ts-node/register/transpile-only')

const repoRoot = path.resolve(__dirname, '..', '..')
const projectDir = fs.mkdtempSync(path.join(repoRoot, 'tmp-watch-smoke-'))
const tsconfigPath = path.join(projectDir, 'tsconfig.json')
const fileCount = 200

const noop = () => {}
console.info = noop

let active = 0
let maxActive = 0
let started = 0
let completed = 0
let settleTimer
let timeoutTimer
let finished = false

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
        }) + '\n',
    )
    process.exit(code)
}

function scheduleFinishIfSettled() {
    if (started > 1 && active === 0) {
        clearTimeout(settleTimer)
        settleTimer = setTimeout(() => finish(0), 300)
    }
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

for (let i = 0; i < fileCount; i++) {
    const dir = path.join(projectDir, 'src', `group${String(i % 10).padStart(2, '0')}`)
    const filePath = path.join(dir, `logic${i}.ts`)

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
    active += 1
    started += 1
    maxActive = Math.max(maxActive, active)
    clearTimeout(settleTimer)

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
