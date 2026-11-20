import { spawnSync } from 'child_process'
import * as fs from 'fs'
import * as os from 'os'
import * as path from 'path'

const repo = path.resolve(__dirname, '../..')

// npm run build:go adds the in-process visitor to this compatibility suite.
const binary = path.join(
    repo,
    'go/bin',
    process.platform + '-' + process.arch,
    'kea-typegen-go' + (process.platform === 'win32' ? '.exe' : ''),
)
const engines = fs.existsSync(binary) ? ['native', 'go'] : ['native']

function runCli(root: string, ...args: string[]) {
    const result = spawnSync(
        process.execPath,
        ['-r', require.resolve('ts-node/register/transpile-only'), path.join(repo, 'src/cli/typegen.ts'), ...args],
        {
            cwd: root,
            encoding: 'utf8',
            env: {
                ...process.env,
                TS_NODE_PROJECT: path.join(repo, 'tsconfig.json'),
                TS_NODE_COMPILER: require.resolve('typescript'),
            },
        },
    )
    if (result.error) throw result.error
    expect(result.stderr).toBe('')
    return result
}

test.each(engines)('%s writes and converges with inferred imports and UTF-16 source edits', testWrite, 30000)

function testWrite(compiler: string) {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-native-'))
    const file = (name: string) => path.join(root, name)
    try {
        fs.mkdirSync(file('node_modules/kea'), { recursive: true })
        fs.writeFileSync(file('node_modules/kea/index.d.ts'), 'export declare function kea<T>(input: unknown): T')
        fs.writeFileSync(file('.kearc'), JSON.stringify({ compiler }))
        fs.writeFileSync(
            file('base.json'),
            JSON.stringify({
                compilerOptions: { target: 'es2026', module: 'nodenext', paths: { '@models': ['./models.ts'] } },
            }),
        )
        fs.writeFileSync(file('tsconfig.json'), JSON.stringify({ extends: './base.json', include: ['*.ts'] }))
        fs.writeFileSync(
            file('models.ts'),
            'export interface Model { label: string }; export declare function model(): Model',
        )
        fs.writeFileSync(file('record.ts'), 'export interface Item { id: number }')
        fs.writeFileSync(file('api.ts'), "export * as ModelAPI from './record'")
        const source = [
            "import { kea } from 'kea'",
            "import { model } from '@models'",
            "import { ModelAPI } from './api'",
            '// 🦜 café',
            'export const logic = kea({ actions: { save: (item: ModelAPI.Item) => ({item}), load: () => model(), payload: (text: string | null) => ({ text }) } })',
        ].join('\n')
        fs.writeFileSync(file('logic.ts'), source)
        fs.writeFileSync(file('second.ts'), source.replace('const logic', 'const second'))
        expect(runCli(root, 'check', '--file', 'logic.ts').status).toBe(1)
        expect(fs.readFileSync(file('logic.ts'), 'utf8')).toBe(source)
        expect(fs.existsSync(file('logicType.ts'))).toBe(false)

        const written = runCli(root, 'write', '--file', 'logic.ts')
        expect(written.status).toBe(0)
        expect(written.stdout).toContain('Native TypeScript 7.1.0-beta')
        expect(fs.existsSync(file('secondType.ts'))).toBe(false)
        expect(fs.readFileSync(file('logic.ts'), 'utf8')).toContain('// 🦜 café\nexport const logic = kea<logicType>')
        const generated = fs.readFileSync(file('logicType.ts'), 'utf8')
        expect(generated).toMatch(/import\(['"](?:\.\/models|@models)['"]\)\.Model/)
        expect(generated).toContain("import type { ModelAPI } from './api'")
        expect(generated).toMatch(/text: string \| null/)
        fs.writeFileSync(file('logic.ts'), source)
        expect(runCli(root, 'check', '--file', 'logic.ts').status).toBe(1)
        expect(runCli(root, 'write').status).toBe(0)
        expect(runCli(root, 'check').status).toBe(0)
        expect(fs.readFileSync(file('logicType.ts'), 'utf8')).toBe(generated)

        expect(runCli(root, 'write', '--inline', '--delete').status).toBe(0)
        expect(fs.existsSync(file('logicType.ts'))).toBe(false)
        expect(fs.readFileSync(file('logic.ts'), 'utf8')).toContain('MakeLogicType')
        expect(runCli(root, 'check', '--inline').status).toBe(0)

        const js = "import { kea } from 'kea'\nexport const store = kea({ actions: { save: (n = 1) => ({ n }) } })"
        fs.writeFileSync(file('store.mjs'), js)
        expect(runCli(root, 'write', '--file', 'store.mjs', '--inline', '--delete').status).toBe(0)
        expect(fs.readFileSync(file('store.mjs'), 'utf8')).toBe(js)
        expect(fs.readFileSync(file('storeType.ts'), 'utf8')).toContain('n: number')
    } finally {
        fs.rmSync(root, { recursive: true, force: true })
    }
}

test.each(engines)('%s generates Node ESM imports into a separate types directory', testEsm, 15000)

function testEsm(compiler: string) {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-native-esm-'))
    const file = (name: string) => path.join(root, name)
    try {
        fs.mkdirSync(file('src'))
        fs.mkdirSync(file('node_modules/kea'), { recursive: true })
        fs.symlinkSync(
            path.dirname(require.resolve('typescript-native/package.json')),
            file('node_modules/typescript'),
            'junction',
        )
        fs.writeFileSync(file('node_modules/kea/package.json'), JSON.stringify({ name: 'kea', types: './index.d.ts' }))
        fs.writeFileSync(file('node_modules/kea/index.d.ts'), 'export declare function kea<T>(input: unknown): T')
        fs.writeFileSync(file('package.json'), JSON.stringify({ type: 'module' }))
        fs.writeFileSync(
            file('tsconfig.json'),
            JSON.stringify({ compilerOptions: { module: 'nodenext' }, include: ['src', 'generated'] }),
        )
        fs.writeFileSync(
            file('src/model.ts'),
            'export interface Item { n: number }; export declare function make(): Item',
        )
        fs.writeFileSync(
            file('src/demo.mts'),
            "import { kea } from 'kea'\nimport { make } from './model.js'\nexport const demo = kea({ actions: { load: () => make() } })",
        )
        const args = ['--root', 'src', '--types', 'generated', '--delete']
        args.push('--compiler', compiler)
        const result = runCli(root, 'write', ...args)
        expect(result.status).toBe(0)
        expect(result.stdout).toContain('Native TypeScript')
        expect(fs.readFileSync(file('src/demo.mts'), 'utf8')).toContain('../generated/demoType.js')
        expect(fs.readFileSync(file('generated/demoType.ts'), 'utf8')).toContain('../src/model.js')
        expect(runCli(root, 'check', ...args).status).toBe(0)
    } finally {
        fs.rmSync(root, { recursive: true, force: true })
    }
}
