// Run via npm so the user's selected CLI handles browser login and 2FA on every platform.
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { execFileSync } = require('node:child_process')
const { createInterface } = require('node:readline/promises')
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
    if (response.ok) return false
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
        return true
    } finally {
        fs.rmSync(dir, { recursive: true, force: true })
    }
}

async function hasPublisher(name) {
    // npm trust list --json prints one object per configuration, separated by blank lines.
    let output
    try {
        output = npm(['trust', 'list', name, '--json'], true).trim()
    } catch (error) {
        // npm's browser/OTP prompt requires a terminal on both stdin and stdout.
        if (!/\bEOTP\b/.test(String(error.stdout))) throw error
        npm(['trust', 'list', name])
        const prompt = createInterface({ input: process.stdin, output: process.stdout })
        try {
            const answer = await prompt.question(
                `Does ${name} already list ${repo} / release.yml with publish access and a blank environment? [y/N] `,
            )
            return /^y(es)?$/i.test(answer.trim())
        } finally {
            prompt.close()
        }
    }
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
        const created = name !== 'kea-typegen' && (await stageMissingWorker(name))
        if (created || !(await hasPublisher(name))) {
            npm(['trust', 'github', name, '--repo=' + repo, '--file=release.yml', '--allow-publish', '--yes'])
            await delay(2000)
        }
    }
}

main().catch((error) => {
    console.error(error.message)
    process.exitCode = 1
})
