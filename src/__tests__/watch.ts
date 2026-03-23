import { spawn } from 'child_process'
import * as path from 'path'

test(
    'watch mode does not overlap successive typegen passes',
    async () => {
        const repoRoot = path.resolve(__dirname, '..', '..')
        const scriptPath = path.join(repoRoot, 'src/test-support/watch-mode-smoke.js')

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

        expect(payload.started).toBeGreaterThan(1)
        expect(payload.completed).toBeGreaterThan(0)
        expect(payload.maxActive).toBe(1)
        expect(result.stderr).toBe('')
    },
    30000,
)
