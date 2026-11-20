const fs = require('node:fs')
const path = require('node:path')
const { execFileSync } = require('node:child_process')
const pkg = path.dirname(require.resolve('kea-typegen/package.json', { paths: [process.cwd()] }))
const cli = path.join(pkg, 'dist/src/cli/typegen.js')
const plugin = process.argv[2]
fs.writeFileSync('tsconfig.json', '{"compilerOptions":{"module":"nodenext"},"files":["counter.ts"]}')
const fixtures = {
    'kea-forms': {
        input: "import { forms } from 'kea-forms'\nexport const counter = kea([forms({profile:{defaults:{email:''}}})])",
        includes: ['submitProfile:', 'email: string'],
    },
    'kea-saga': {
        input: "import { workers } from 'kea-saga'\nexport const counter = kea([workers({sync: function* () {}}), workers({save: function* () {}})])",
        includes: ['workers:', 'sync: Saga', 'save: Saga', "from 'redux-saga'"],
    },
}
if (plugin && !Object.hasOwn(fixtures, plugin)) throw Error('Unknown plugin ' + plugin)
const fixture = fixtures[plugin] ?? {
    input: 'export const counter = kea({actions:{ increment: (n: number) => ({n}) }})',
    includes: ['n: number'],
}
const input = 'declare function kea<T>(input: unknown): T\n' + fixture.input + '\n'
fs.writeFileSync('counter.ts', input)
const log = execFileSync(process.execPath, [cli, 'write', '--compiler', 'go', '--verbose'], { encoding: 'utf8' })
const types = fs.readFileSync('counterType.ts', 'utf8')
if (plugin && !log.includes('JavaScript plugin compatibility generated 0.')) {
    throw Error('The packed ' + plugin + ' visitor delegated to JavaScript: ' + log)
}
for (const expected of fixture.includes) {
    if (!types.includes(expected)) throw Error('Missing ' + expected + ' in generated declaration: ' + types)
}
execFileSync(process.execPath, [cli, 'check', '--compiler', 'go'], { stdio: 'inherit' })
