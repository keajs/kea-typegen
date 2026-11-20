// Shared by the kea-typegen workflow and its kea-forms caller.
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const { execFileSync } = require('node:child_process')
const { scope, targets } = require('./platform.cjs')
const root = process.cwd()
const run = (command, args) => execFileSync(command, args, { encoding: 'utf8' }).trim()
const git = (...args) => run('git', args)
const read = (file) => fs.readFileSync(file, 'utf8')
const write = (file, value) => fs.writeFileSync(file, value)
const manifest = () => JSON.parse(read('package.json'))
const output = (key, value) =>
    process.env.GITHUB_OUTPUT && fs.appendFileSync(process.env.GITHUB_OUTPUT, key + '=' + value + '\n')

function changesSinceRelease(previousVersion) {
    const previousTag = 'v' + previousVersion
    const tagged = git('tag', '--list', previousTag)
    const base =
        tagged || git('log', '-1', '--format=%H', '-S', '"version": "' + previousVersion + '"', '--', 'package.json')
    if (!base) throw Error('Cannot find the previous release in git history')
    return git('log', '--no-merges', '--format=- %s (%h)', base + '..HEAD') || '- Rebuild the published package.'
}

function prepare(dir, requested) {
    const pkg = manifest()
    const semver = require(require.resolve('semver', { paths: [root] }))
    const [introduction, ...sections] = read('CHANGELOG.md').split(/^## /m)
    const prepared = sections[0]?.split(/\r?\n/, 1)[0] === pkg.version + ' - Unreleased'
    const version = requested || (prepared ? pkg.version : semver.inc(pkg.version, 'minor'))
    const usesPreparedVersion = prepared && version === pkg.version
    if (semver.valid(version) !== version || (!usesPreparedVersion && !semver.gt(version, pkg.version)))
        throw Error('Expected a new semantic version greater than ' + pkg.version)
    const isCore = pkg.name === 'kea-typegen'
    const platforms = isCore ? targets() : []
    const notes = prepared ? sections.shift().split('\n').slice(1).join('\n').trim() : changesSinceRelease(pkg.version)
    const date = process.env.RELEASE_DATE || new Date().toISOString().slice(0, 10)
    fs.mkdirSync(dir, { recursive: true })
    sections.unshift(version + ' - ' + date + '\n\n' + notes + '\n\n')
    write('CHANGELOG.md', [introduction, ...sections].join('## '))
    write('package.json', read('package.json').replace(/("version"\s*:\s*)"[^"]+"/, '$1"' + version + '"'))
    for (const file of ['package.json', 'CHANGELOG.md']) fs.copyFileSync(file, path.join(dir, file))
    write(path.join(dir, 'notes.md'), notes + '\n')
    write(
        path.join(dir, 'plan.json'),
        JSON.stringify({ name: pkg.name, version, platforms, sha: git('rev-parse', 'HEAD') }),
    )
    output('version', version)
    output('core', String(isCore))
    output('platforms', JSON.stringify(platforms))
    console.log(pkg.name + '@' + version)
}

function pack(dir) {
    const plan = JSON.parse(read(path.join(dir, 'plan.json')))
    const original = read('package.json')
    try {
        if (plan.platforms.length) {
            const pkg = manifest()
            pkg.optionalDependencies = Object.fromEntries(plan.platforms.map((p) => [scope + p, plan.version]))
            write('package.json', JSON.stringify(pkg, null, 4) + '\n')
        }
        const result = JSON.parse(run('npm', ['pack', '--ignore-scripts', '--json', '--pack-destination', dir]))[0]
        const files = new Set(result.files.map((f) => f.path))
        const expected = plan.platforms.length
            ? [
                  'dist/src/cli/typegen.js',
                  'dist/src/go.js',
                  'go/go.mod',
                  'go/build.cjs',
                  'plugin/go.mod',
                  'plugin/plugin.go',
              ]
            : ['lib/index.js', 'lib/typegen.js', 'lib/typegen.json', 'lib/typegen.wasm']
        for (const file of expected) if (!files.has(file)) throw Error('Missing package file: ' + file)
    } finally {
        write('package.json', original)
    }
}

async function publish(dir) {
    const plan = JSON.parse(read(path.join(dir, 'plan.json')))
    const { name, version, platforms, sha } = plan
    const tag = 'v' + version
    const registry = 'https://registry.npmjs.org/'
    const archives = platforms
        .map((p) => scope + p)
        .concat(name)
        .map((pkg) => {
            const filename = pkg.replace(/^@/, '').replace('/', '-') + '-' + version + '.tgz'
            const file = path.join(dir, filename)
            const integrity = 'sha512-' + crypto.createHash('sha512').update(fs.readFileSync(file)).digest('base64')
            return { pkg, file, integrity, exists: false }
        })
    // npm versions are immutable. On a retry only accept bytes from the tested artifacts.
    for (const a of archives) {
        const response = await fetch(registry + encodeURIComponent(a.pkg) + '/' + version)
        if (response.status === 404) continue
        if (!response.ok) throw Error('npm registry returned ' + response.status + ' for ' + a.pkg)
        if ((await response.json()).dist.integrity !== a.integrity)
            throw Error('Published archive differs: ' + a.pkg + '@' + version)
        a.exists = true
    }
    if (process.env.GITHUB_REF !== 'refs/heads/main' || git('rev-parse', 'HEAD') !== sha)
        throw Error('Release must use the tested main commit')
    const remoteTag = git('ls-remote', 'origin', 'refs/tags/' + tag).split(/\s+/)[0]
    git('config', 'user.name', 'github-actions[bot]')
    git('config', 'user.email', '41898282+github-actions[bot]@users.noreply.github.com')
    for (const file of ['CHANGELOG.md', 'package.json']) fs.copyFileSync(path.join(dir, file), file)
    if (remoteTag) {
        git('fetch', 'origin', 'main')
        if (JSON.parse(git('show', 'FETCH_HEAD:package.json')).version !== version)
            throw Error('A newer release has superseded this retry')
        git('fetch', 'origin', 'refs/tags/' + tag)
        if (git('rev-parse', 'FETCH_HEAD^') !== sha || git('diff', 'FETCH_HEAD', '--', 'package.json', 'CHANGELOG.md'))
            throw Error('Tag belongs to another release')
    } else {
        if (git('ls-remote', 'origin', 'refs/heads/main').split(/\s+/)[0] !== sha)
            throw Error('main advanced; dispatch a fresh release')
        git('add', '--', 'package.json', 'CHANGELOG.md')
        git('commit', '-m', 'Release ' + name + ' ' + version)
        git('tag', tag)
        const refs = ['HEAD:refs/heads/main', 'refs/tags/' + tag]
        if (platforms.length && !git('ls-remote', 'origin', 'refs/tags/plugin/v1.0.0')) {
            git('tag', 'plugin/v1.0.0')
            refs.push('refs/tags/plugin/v1.0.0')
        }
        git('push', '--atomic', 'origin', ...refs)
    }
    for (const a of archives) {
        if (!a.exists)
            execFileSync(
                'npm',
                [
                    'publish',
                    a.file,
                    '--ignore-scripts',
                    '--access',
                    'public',
                    '--provenance',
                    '--tag',
                    version.includes('-') ? 'next' : 'latest',
                ],
                { stdio: 'inherit' },
            )
    }
    const release = await fetch(
        'https://api.github.com/repos/' + process.env.GITHUB_REPOSITORY + '/releases/tags/' + tag,
        {
            headers: { Authorization: 'Bearer ' + process.env.GH_TOKEN, 'User-Agent': 'kea-release' },
        },
    )
    if (release.status === 404)
        run('gh', [
            'release',
            'create',
            tag,
            '--verify-tag',
            '--title',
            name + ' ' + version,
            '--notes-file',
            path.join(dir, 'notes.md'),
            ...(version.includes('-') ? ['--prerelease'] : []),
        ])
    else if (!release.ok) throw Error('GitHub release lookup failed: ' + release.status)
}
const [cmd, dir, arg] = process.argv.slice(2)
if (cmd === 'prepare') prepare(path.resolve(dir), arg)
else if (cmd === 'pack') pack(path.resolve(dir))
else if (cmd === 'publish')
    publish(path.resolve(dir)).catch((err) => {
        console.error(err)
        process.exitCode = 1
    })
else throw Error('Expected prepare, pack or publish')
