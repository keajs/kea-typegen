const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { execFileSync } = require('node:child_process')

test('a partial release resumes from the tested archives and rejects conflicting registry bytes', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-release-test-'))
    try {
        const repo = path.join(dir, 'repo'),
            remote = path.join(dir, 'remote.git'),
            artifacts = path.join(dir, 'packages')
        fs.mkdirSync(repo)
        const git = (...args) =>
            execFileSync('git', args, { cwd: repo, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
        git('init', '-b', 'main')
        git('config', 'user.email', 'test@example.com')
        git('config', 'user.name', 'Test')
        fs.writeFileSync(path.join(repo, 'package.json'), '{ "name": "kea-forms", "version": "2.7.1" }\n')
        fs.writeFileSync(path.join(repo, 'CHANGELOG.md'), '# Changes\n\n## 2.7.1\n')
        git('add', 'package.json', 'CHANGELOG.md')
        git('commit', '-m', 'Release 2.7.1')
        git('tag', 'v2.7.1')
        git('commit', '--allow-empty', '-m', 'Add portable visitors')
        git('init', '--bare', remote)
        git('remote', 'add', 'origin', remote)
        git('push', 'origin', 'main', 'v2.7.1')
        const sha = git('rev-parse', 'HEAD')
        fs.symlinkSync(path.resolve(__dirname, '../node_modules'), path.join(repo, 'node_modules'), 'junction')
        const preload = path.join(dir, 'offline.cjs')
        // Exercise real git pushes against a local bare repo. Intercept every registry and GitHub operation.
        fs.writeFileSync(
            preload,
            `
const fs=require('node:fs'),crypto=require('node:crypto'),cp=require('node:child_process')
const state=process.env.REGISTRY_STATE
const read=()=>JSON.parse(fs.readFileSync(state))
global.fetch=async (url)=>{
    if(url.startsWith('https://api.github.com/')) return {status:404}
    const pkg=decodeURIComponent(new URL(url).pathname.slice(1).split('/')[0])
    const integrity=read()[pkg]
    return integrity ? {status:200,ok:true,json:async()=>({dist:{integrity}})} : {status:404}
}
const exec=cp.execFileSync
cp.execFileSync=function(cmd,args,...rest){
    if(cmd==='git') return exec(cmd,args,...rest)
    if(cmd==='gh') return ''
    if(cmd!=='npm' || args[0]!=='publish') throw Error('Unexpected command '+cmd)
    const pkg=args[1].includes('keajs-kea-typegen-linux-x64')?'@keajs/kea-typegen-linux-x64':'kea-forms'
    const data=read()
    if(data[pkg]) throw Error('duplicate publish')
    if(process.env.FAIL_MAIN && pkg==='kea-forms') throw Error('simulated registry failure')
    data[pkg]='sha512-'+crypto.createHash('sha512').update(fs.readFileSync(args[1])).digest('base64')
    fs.writeFileSync(state,JSON.stringify(data))
}
`,
        )
        const state = path.join(dir, 'registry.json')
        fs.writeFileSync(state, '{}')
        const env = {
            ...process.env,
            GITHUB_REF: 'refs/heads/main',
            GITHUB_REPOSITORY: 'example/fixture',
            REGISTRY_STATE: state,
            GITHUB_OUTPUT: '',
        }
        const release = (cmd, extra = {}, args = []) =>
            execFileSync(
                process.execPath,
                ['-r', preload, path.join(__dirname, 'release.cjs'), cmd, artifacts, ...args],
                { cwd: repo, env: { ...env, ...extra }, stdio: 'pipe' },
            )
        assert.throws(() => release('prepare', {}, ['2.7.1']), /Expected a new semantic version/)
        release('prepare')
        const plan = JSON.parse(fs.readFileSync(path.join(artifacts, 'plan.json')))
        assert.equal(plan.version, '2.8.0')
        assert.match(
            fs.readFileSync(path.join(artifacts, 'CHANGELOG.md'), 'utf8'),
            /^# Changes\n\n## 2\.8\.0[\s\S]*Add portable visitors/,
        )
        plan.platforms = ['linux-x64']
        fs.writeFileSync(path.join(artifacts, 'plan.json'), JSON.stringify(plan))
        fs.writeFileSync(path.join(artifacts, 'keajs-kea-typegen-linux-x64-2.8.0.tgz'), 'worker')
        fs.writeFileSync(path.join(artifacts, 'kea-forms-2.8.0.tgz'), 'main')
        assert.throws(() => release('publish', { FAIL_MAIN: '1' }), /simulated registry failure/)
        git('switch', '--detach', sha)
        assert.deepEqual(Object.keys(JSON.parse(fs.readFileSync(state))), ['@keajs/kea-typegen-linux-x64'])
        release('publish')
        assert.equal(Object.keys(JSON.parse(fs.readFileSync(state))).length, 2)
        fs.writeFileSync(path.join(artifacts, 'kea-forms-2.8.0.tgz'), 'different')
        assert.throws(() => release('publish'), /Published archive differs/)
    } finally {
        fs.rmSync(dir, { recursive: true, force: true })
    }
})

test('a reviewed version is released once with its notes and the project changelog header', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-prepared-release-'))
    try {
        fs.symlinkSync(path.resolve(__dirname, '../node_modules'), path.join(dir, 'node_modules'), 'junction')
        execFileSync('git', ['init', '-b', 'main'], { cwd: dir })
        const git = (...args) => execFileSync('git', args, { cwd: dir })
        const changelog = path.join(dir, 'CHANGELOG.md')
        const details = '- Ships the portable visitor.'
        for (const [name, version, input] of [
            ['kea-typegen', '4.0.0', ['4.0.0']],
            ['kea-forms', '3.3.0', []],
        ]) {
            fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({ name, version }))
            fs.writeFileSync(changelog, `# Changes\n\n## ${version} - Unreleased\n\n${details}\n\n## 3.2.0\n`)
            git('add', 'package.json', 'CHANGELOG.md')
            git('-c', 'user.name=Test', '-c', 'user.email=test@example.com', 'commit', '-m', 'Prepare ' + name)
            const prepare = (...args) =>
                execFileSync(
                    process.execPath,
                    [path.join(__dirname, 'release.cjs'), 'prepare', dir + '/artifacts', ...args],
                    { cwd: dir, env: { ...process.env, RELEASE_DATE: '2026-11-20' }, stdio: 'pipe' },
                )
            prepare(...input)
            assert.equal(
                fs.readFileSync(changelog, 'utf8'),
                `# Changes\n\n## ${version} - 2026-11-20\n\n${details}\n\n## 3.2.0\n`,
            )
            assert.equal(fs.readFileSync(path.join(dir, 'artifacts', 'notes.md'), 'utf8'), details + '\n')
            assert.throws(() => prepare(version), /Expected a new semantic version/)
        }
    } finally {
        fs.rmSync(dir, { recursive: true, force: true })
    }
})
