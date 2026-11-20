const fs = require('node:fs')
const path = require('node:path')
const { execFileSync } = require('node:child_process')
const pkg = path.dirname(require.resolve('kea-typegen/package.json', { paths: [process.cwd()] }))
const cli = path.join(pkg, 'dist/src/cli/typegen.js')
const forms = process.argv.includes('--forms')
fs.writeFileSync('tsconfig.json', '{"compilerOptions":{"module":"nodenext"},"files":["counter.ts"]}')
const input = forms
    ? "import { forms } from 'kea-forms'\ndeclare function kea<T>(input: unknown): T\nexport const counter = kea([forms({profile:{defaults:{email:''}}})])\n"
    : 'declare function kea<T>(input: unknown): T\nexport const counter = kea({actions:{ increment: (n: number) => ({n}) }})\n'
fs.writeFileSync('counter.ts', input)
const log = execFileSync(process.execPath, [cli, 'write', '--compiler', 'go', '--verbose'], { encoding: 'utf8' })
const types = fs.readFileSync('counterType.ts', 'utf8')
if (
    forms &&
    (!log.includes('JavaScript plugin compatibility generated 0.') ||
        !types.includes('submitProfile:') ||
        !types.includes('email: string'))
) {
    throw Error('The packaged forms module did not generate its inferred types: ' + log)
}
if (!forms && !types.includes('n: number')) throw Error('The packed worker did not infer the action payload')
execFileSync(process.execPath, [cli, 'check', '--compiler', 'go'], { stdio: 'inherit' })
