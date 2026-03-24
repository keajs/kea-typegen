import { spawn } from 'child_process'
import * as fs from 'fs'
import * as os from 'os'
import * as path from 'path'
import * as ts from 'typescript'
import { AppOptions } from '../types'
import { visitProgram } from '../visit/visit'
import { writeTypeImports } from '../write/writeTypeImports'

test(
    'write mode recreates the TypeScript program from scratch between passes',
    async () => {
        const repoRoot = path.resolve(__dirname, '..', '..')
        const scriptPath = path.join(repoRoot, 'src/test-support/write-mode-smoke.js')

        const result = await new Promise<{ code: number | null; signal: NodeJS.Signals | null; stdout: string; stderr: string }>(
            (resolve, reject) => {
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
            },
        )

        expect(result.signal).toBeNull()
        expect(result.code).toBe(0)

        const payload = JSON.parse(result.stdout.trim())

        expect(payload.createProgramCalls).toBeGreaterThan(1)
        expect(payload.reusedOldProgram).toBe(false)
        expect(result.stderr).toBe('')
    },
    30000,
)

test('writeTypeImports adds missing type import and kea generic', async () => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-write-imports-'))

    try {
        const logicDir = path.join(tempDir, 'src')
        const logicPath = path.join(logicDir, 'logic.ts')
        const keaDtsPath = path.join(tempDir, 'node_modules', 'kea', 'index.d.ts')

        fs.mkdirSync(path.dirname(keaDtsPath), { recursive: true })
        fs.mkdirSync(logicDir, { recursive: true })

        fs.writeFileSync(keaDtsPath, 'export function kea<T = any>(input: any): T\n')
        fs.writeFileSync(
            logicPath,
            [
                "import { kea } from 'kea'",
                '',
                'export const logic = kea({',
                '    actions: () => ({',
                '        setValue: (value: string) => ({ value }),',
                '    }),',
                '})',
                '',
            ].join('\n'),
        )

        const program = ts.createProgram([logicPath], {
            module: ts.ModuleKind.CommonJS,
            moduleResolution: ts.ModuleResolutionKind.NodeJs,
            target: ts.ScriptTarget.ES2020,
            skipLibCheck: true,
        })

        const appOptions: AppOptions = {
            rootPath: logicDir,
            typesPath: logicDir,
            log: () => {},
        }

        const [parsedLogic] = visitProgram(program, appOptions)
        await writeTypeImports(appOptions, program, logicPath, [parsedLogic], [parsedLogic])

        const writtenLogic = fs.readFileSync(logicPath, 'utf8')

        expect(writtenLogic).toContain("import type { logicType } from './logicType'")
        expect(writtenLogic).toContain('export const logic = kea<logicType>({')
    } finally {
        fs.rmSync(tempDir, { recursive: true, force: true })
    }
})

test('writeTypeImports replaces existing kea generic without leaving a trailing >', async () => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-write-existing-generic-'))

    try {
        const logicDir = path.join(tempDir, 'src')
        const logicPath = path.join(logicDir, 'logic.ts')
        const logicTypePath = path.join(logicDir, 'logicType.ts')
        const keaDtsPath = path.join(tempDir, 'node_modules', 'kea', 'index.d.ts')

        fs.mkdirSync(path.dirname(keaDtsPath), { recursive: true })
        fs.mkdirSync(logicDir, { recursive: true })

        fs.writeFileSync(keaDtsPath, 'export function kea<T = any>(input: any): T\n')
        fs.writeFileSync(logicTypePath, 'export interface logicType {}\n')
        fs.writeFileSync(
            logicPath,
            [
                "import type { logicType } from './logicType'",
                "import { kea, actions } from 'kea'",
                '',
                'export const logic = kea<logicType<string>>([',
                '    actions({',
                '        setValue: (value: string) => ({ value }),',
                '    }),',
                '])',
                '',
            ].join('\n'),
        )

        const program = ts.createProgram([logicPath], {
            module: ts.ModuleKind.CommonJS,
            moduleResolution: ts.ModuleResolutionKind.NodeJs,
            target: ts.ScriptTarget.ES2020,
            skipLibCheck: true,
        })

        const appOptions: AppOptions = {
            rootPath: logicDir,
            typesPath: logicDir,
            log: () => {},
        }

        const [parsedLogic] = visitProgram(program, appOptions)
        await writeTypeImports(appOptions, program, logicPath, [parsedLogic], [parsedLogic])

        const writtenLogic = fs.readFileSync(logicPath, 'utf8')

        expect(writtenLogic).toContain("import type { logicType } from './logicType'")
        expect(writtenLogic).toContain('export const logic = kea<logicType>([')
        expect(writtenLogic).not.toContain('kea<logicType>>([')
    } finally {
        fs.rmSync(tempDir, { recursive: true, force: true })
    }
})
