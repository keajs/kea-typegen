// Run via npm so the user's selected CLI handles browser login and 2FA on every platform.
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { execFileSync } = require('node:child_process')
const { setTimeout: delay } = require('node:timers/promises')
const { scope, targets } = require('./platform.cjs')

const registry = 'https://registry.npmjs.org'
const repo = 'keajs/kea-typegen'

function npm(args, capture = false) {
    return execFileSync(process.execPath, [process.env.npm_execpath, ...args, '--registry=' + registry], {
        encoding: 'utf8',
        stdio: ['inherit', capture ? 'pipe' : 'inherit', 'inherit'],
    })
}

async function stageMissingWorker(name) {
    const response = await fetch(registry + '/' + encodeURIComponent(name), { method: 'HEAD' })
    if (response.ok) return
    if (response.status !== 404) throw Error('Looking up ' + name + ' returned ' + response.status)
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-npm-bootstrap-'))
    try {
        const manifest = {
            name,
            version: '0.0.0-bootstrap.0',
            description: 'Reserve the platform package name for the kea-typegen release workflow.',
            license: 'MIT',
            repository: 'github:' + repo,
        }
        fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify(manifest))
        npm(['stage', 'publish', dir, '--access=public', '--tag=bootstrap', '--ignore-scripts'])
    } finally {
        fs.rmSync(dir, { recursive: true, force: true })
    }
}

function hasPublisher(name) {
    // npm trust list --json prints one object per configuration, separated by blank lines.
    const output = npm(['trust', 'list', name, '--json'], true).trim()
    const configs = output ? output.split(/\r?\n\s*\r?\n/).flatMap((item) => JSON.parse(item)) : []
    return configs.some(
        (config) =>
            config.type === 'github' &&
            config.repository === repo &&
            config.file === 'release.yml' &&
            !config.environment &&
            config.permissions?.includes('createPackage'),
    )
}

async function main() {
    const packages = ['kea-typegen', ...targets().map((target) => scope + target)]
    const args = process.argv.slice(2)
    if (args.length > 1 || (args.length === 1 && args[0] !== '--list'))
        throw Error('Usage: npm run release:setup [-- --list]')
    if (args[0] === '--list') {
        console.log(packages.join('\n'))
        return
    }
    const version = npm(['--version'], true).trim()
    if (!require('semver').satisfies(version, '>=11.15.0')) throw Error('Run with npm 11.15.0 or newer.')
    npm(['whoami'])
    for (const name of packages) {
        console.log(name)
        if (name !== 'kea-typegen') await stageMissingWorker(name)
        if (!hasPublisher(name)) {
            npm(['trust', 'github', name, '--repo=' + repo, '--file=release.yml', '--allow-publish', '--yes'])
            await delay(2000)
        }
    }
}

main().catch((error) => {
    console.error(error.message)
    process.exitCode = 1
})
