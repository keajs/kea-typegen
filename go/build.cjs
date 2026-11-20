const { execFileSync } = require('node:child_process')
const { mkdirSync } = require('node:fs')
const path = require('node:path')

const dir = path.join(__dirname, 'bin', process.platform + '-' + process.arch)
mkdirSync(dir, { recursive: true })
execFileSync(
    'go',
    ['build', '-trimpath', '-o', path.join(dir, 'kea-typegen-go' + (process.platform === 'win32' ? '.exe' : '')), '.'],
    {
        cwd: __dirname,
        stdio: 'inherit',
    },
)
