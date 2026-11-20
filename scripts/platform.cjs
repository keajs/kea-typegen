const fs = require('node:fs')
const path = require('node:path')
const os = require('node:os')
const { execFileSync } = require('node:child_process')

const scope = '@keajs/kea-typegen-'

function targets() {
    const upstream = require('typescript-native/package.json').optionalDependencies
    return Object.keys(upstream).map((name) => name.replace('@typescript/typescript-', ''))
}

function packPlatform(target, version, output) {
    const [platform, cpu] = target.split('-')
    const goos = { win32: 'windows', sunos: 'solaris' }[platform] || platform
    const goarch =
        { x64: 'amd64', ia32: '386', mips64el: 'mips64le', ppc64: platform === 'linux' ? 'ppc64le' : 'ppc64' }[cpu] ||
        cpu
    const env = { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: '0' }
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-platform-'))
    const binary = 'kea-typegen-go' + (platform === 'win32' ? '.exe' : '')
    const go = (...args) => execFileSync('go', args, { cwd: 'go', env, encoding: 'utf8' })
    try {
        go('build', '-trimpath', '-buildvcs=false', '-ldflags=-s -w', '-o', path.join(dir, binary), '.')
        const notices = []
        const modules = go('list', '-deps', '-f', '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}', '.')
        const goRoot = go('env', 'GOROOT').trim()
        for (const entry of new Set(['Go|' + goRoot, ...modules.trim().split(/\r?\n/)])) {
            if (!entry) continue
            const [name, location] = entry.split('|')
            const licenses = fs
                .readdirSync(location)
                .filter((f) => /^(LICENSE|LICENCE|COPYING|NOTICE)([.-]|$)/i.test(f))
            if (name === 'github.com/microsoft/TypeScript/tsc') {
                licenses.push('internal/bundled/CopyrightNotice.txt', 'internal/fswatch/LICENSE')
            }
            for (const file of licenses) {
                if (fs.statSync(path.join(location, file)).isFile())
                    notices.push(name + ' — ' + file + '\n' + fs.readFileSync(path.join(location, file), 'utf8'))
            }
        }
        fs.writeFileSync(path.join(dir, 'NOTICE.txt'), notices.join('\n\n'))
        const manifest = {
            name: scope + target,
            version,
            os: [platform],
            cpu: [cpu],
            license: 'MIT',
            description: 'Native kea-typegen worker for ' + target,
            repository: require(path.resolve('package.json')).repository,
        }
        fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify(manifest, null, 2) + '\n')
        fs.mkdirSync(output, { recursive: true })
        execFileSync('npm', ['pack', '--ignore-scripts', '--pack-destination', output], { cwd: dir, stdio: 'inherit' })
    } finally {
        fs.rmSync(dir, { recursive: true, force: true })
    }
}

module.exports = { scope, targets }
if (require.main === module) packPlatform(process.argv[2], process.argv[3], path.resolve(process.argv[4]))
