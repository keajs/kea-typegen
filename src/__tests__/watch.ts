import { spawn } from 'child_process'
import * as fs from 'fs'
import * as os from 'os'
import * as path from 'path'
import * as ts from 'typescript'
import { planWatchTypegenPass } from '../watch'

function writeFile(filePath: string, source: string): void {
    fs.mkdirSync(path.dirname(filePath), { recursive: true })
    fs.writeFileSync(filePath, source)
}

function createProgram(fileNames: string[]): ts.Program {
    return ts.createProgram(fileNames, {
        module: ts.ModuleKind.CommonJS,
        moduleResolution: ts.ModuleResolutionKind.NodeJs,
        target: ts.ScriptTarget.ES2020,
        skipLibCheck: true,
    })
}

test('watch mode does not overlap successive typegen passes', async () => {
    const repoRoot = path.resolve(__dirname, '..', '..')
    const scriptPath = path.join(repoRoot, 'src/test-support/watch-mode-smoke.js')

    const result = await new Promise<{
        code: number | null
        signal: NodeJS.Signals | null
        stdout: string
        stderr: string
    }>((resolve, reject) => {
        const child = spawn(process.execPath, [scriptPath], {
            cwd: repoRoot,
            env: { ...process.env, FORCE_COLOR: '0' },
        })

        let stdout = ''
        let stderr = ''

        child.stdout.on('data', (chunk) => {
            stdout += chunk.toString()
        })

        child.stderr.on('data', (chunk) => {
            stderr += chunk.toString()
        })

        child.on('error', reject)
        child.on('close', (code, signal) => resolve({ code, signal, stdout, stderr }))
    })

    expect(result.signal).toBeNull()
    expect(result.code).toBe(0)

    const payload = JSON.parse(result.stdout.trim())

    expect(payload.started).toBeGreaterThan(1)
    expect(payload.completed).toBeGreaterThan(0)
    expect(payload.maxActive).toBe(1)
    expect(payload.incrementalAfterEdit).toBe(true)
    expect(payload.parsedLogicCountsAfterEdit).toContain(1)
    expect(payload.sourceFilePathsAfterEdit.some(Boolean)).toBe(true)
    expect(result.stderr).toBe('')
}, 30000)

test('watch planner limits a direct logic edit to the changed logic and dependent logics', () => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-watch-direct-logic-'))

    try {
        const logicDir = path.join(tempDir, 'src')
        const firstLogicPath = path.join(logicDir, 'firstLogic.ts')
        const secondLogicPath = path.join(logicDir, 'secondLogic.ts')
        const unrelatedLogicPath = path.join(logicDir, 'unrelatedLogic.ts')
        const keaDtsPath = path.join(tempDir, 'node_modules', 'kea', 'index.d.ts')

        writeFile(keaDtsPath, 'export function kea<T = any>(input: any): T\n')
        writeFile(
            firstLogicPath,
            [
                "import { kea } from 'kea'",
                '',
                'export const firstLogic = kea({',
                '    actions: () => ({ first: true }),',
                '})',
                '',
            ].join('\n'),
        )
        writeFile(
            secondLogicPath,
            [
                "import { kea } from 'kea'",
                "import { firstLogic } from './firstLogic'",
                '',
                'export const secondLogic = kea({',
                '    connect: { values: [firstLogic, ["first"]] },',
                '})',
                '',
            ].join('\n'),
        )
        writeFile(
            unrelatedLogicPath,
            [
                "import { kea } from 'kea'",
                '',
                'export const unrelatedLogic = kea({',
                '    actions: () => ({ unrelated: true }),',
                '})',
                '',
            ].join('\n'),
        )

        const program = createProgram([firstLogicPath, secondLogicPath, unrelatedLogicPath])
        const plan = planWatchTypegenPass(program, [
            { fileName: firstLogicPath, eventKind: ts.FileWatcherEventKind.Changed },
        ])

        expect(plan).toEqual({
            kind: 'files',
            sourceFilePaths: [firstLogicPath, secondLogicPath].sort(),
            reason: '2 affected Kea logic files',
        })
    } finally {
        fs.rmSync(tempDir, { recursive: true, force: true })
    }
})

test('watch planner regenerates logics affected by a helper type edit', () => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-watch-helper-'))

    try {
        const logicDir = path.join(tempDir, 'src')
        const helperPath = path.join(logicDir, 'helper.ts')
        const logicPath = path.join(logicDir, 'logic.ts')
        const unrelatedLogicPath = path.join(logicDir, 'unrelatedLogic.ts')
        const keaDtsPath = path.join(tempDir, 'node_modules', 'kea', 'index.d.ts')

        writeFile(keaDtsPath, 'export function kea<T = any>(input: any): T\n')
        writeFile(helperPath, 'export interface HelperType { value: string }\n')
        writeFile(
            logicPath,
            [
                "import { kea } from 'kea'",
                "import type { HelperType } from './helper'",
                '',
                'export const logic = kea({',
                '    actions: () => ({ setValue: (value: HelperType) => ({ value }) }),',
                '})',
                '',
            ].join('\n'),
        )
        writeFile(
            unrelatedLogicPath,
            [
                "import { kea } from 'kea'",
                '',
                'export const unrelatedLogic = kea({',
                '    actions: () => ({ unrelated: true }),',
                '})',
                '',
            ].join('\n'),
        )

        const program = createProgram([helperPath, logicPath, unrelatedLogicPath])
        const plan = planWatchTypegenPass(program, [
            { fileName: helperPath, eventKind: ts.FileWatcherEventKind.Changed },
        ])

        expect(plan).toEqual({
            kind: 'files',
            sourceFilePaths: [logicPath],
            reason: '1 affected Kea logic file',
        })
    } finally {
        fs.rmSync(tempDir, { recursive: true, force: true })
    }
})

test('watch planner maps generated type changes back to source logics and dependents', () => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-watch-type-file-'))

    try {
        const logicDir = path.join(tempDir, 'src')
        const firstLogicPath = path.join(logicDir, 'firstLogic.ts')
        const firstLogicTypePath = path.join(logicDir, 'firstLogicType.ts')
        const secondLogicPath = path.join(logicDir, 'secondLogic.ts')
        const keaDtsPath = path.join(tempDir, 'node_modules', 'kea', 'index.d.ts')

        writeFile(keaDtsPath, 'export function kea<T = any>(input: any): T\n')
        writeFile(firstLogicTypePath, 'export interface firstLogicType {}\n')
        writeFile(
            firstLogicPath,
            [
                "import { kea } from 'kea'",
                "import type { firstLogicType } from './firstLogicType'",
                '',
                'export const firstLogic = kea<firstLogicType>({',
                '    actions: () => ({ first: true }),',
                '})',
                '',
            ].join('\n'),
        )
        writeFile(
            secondLogicPath,
            [
                "import { kea } from 'kea'",
                "import { firstLogic } from './firstLogic'",
                '',
                'export const secondLogic = kea({',
                '    connect: { actions: [firstLogic, ["first"]] },',
                '})',
                '',
            ].join('\n'),
        )

        const program = createProgram([firstLogicPath, firstLogicTypePath, secondLogicPath])
        const plan = planWatchTypegenPass(program, [
            { fileName: firstLogicTypePath, eventKind: ts.FileWatcherEventKind.Changed },
        ])

        expect(plan).toEqual({
            kind: 'files',
            sourceFilePaths: [firstLogicPath, secondLogicPath].sort(),
            reason: '2 affected Kea logic files',
        })
    } finally {
        fs.rmSync(tempDir, { recursive: true, force: true })
    }
})

test('watch planner falls back to a full pass for deleted files and resetContext changes', () => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-watch-full-'))

    try {
        const logicDir = path.join(tempDir, 'src')
        const logicPath = path.join(logicDir, 'logic.ts')
        const contextPath = path.join(logicDir, 'context.ts')
        const keaDtsPath = path.join(tempDir, 'node_modules', 'kea', 'index.d.ts')

        writeFile(keaDtsPath, 'export function kea<T = any>(input: any): T\n')
        writeFile(
            logicPath,
            [
                "import { kea } from 'kea'",
                '',
                'export const logic = kea({',
                '    actions: () => ({ value: true }),',
                '})',
                '',
            ].join('\n'),
        )
        writeFile(
            contextPath,
            ["import { resetContext } from 'kea'", '', 'resetContext({ plugins: [] })', ''].join('\n'),
        )

        const program = createProgram([logicPath, contextPath])

        expect(
            planWatchTypegenPass(program, [{ fileName: logicPath, eventKind: ts.FileWatcherEventKind.Deleted }]),
        ).toEqual({ kind: 'full', reason: 'deleted file detected' })

        expect(
            planWatchTypegenPass(program, [{ fileName: contextPath, eventKind: ts.FileWatcherEventKind.Changed }]),
        ).toEqual({
            kind: 'full',
            reason: `resetContext changed: ${path.relative(process.cwd(), contextPath)}`,
        })
    } finally {
        fs.rmSync(tempDir, { recursive: true, force: true })
    }
})
