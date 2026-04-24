const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')

const { compareOutputs } = require('./benchmark-sample-typegen.js')

function writeFile(root, relativePath, contents) {
    const fullPath = path.join(root, relativePath)
    fs.mkdirSync(path.dirname(fullPath), { recursive: true })
    fs.writeFileSync(fullPath, contents)
    return fullPath
}

describe('compareOutputs', () => {
    test('uses logical relative filenames for semantic comparison', () => {
        const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-compare-'))
        const tsDir = path.join(tempRoot, 'posthog-js', 'frontend', 'src', 'scenes')
        const goDir = path.join(tempRoot, 'posthog-go', 'frontend', 'src', 'scenes')
        const importPath = path.join(tsDir, 'sceneTypes').replace(/\\/g, '/')
        const contents = [
            "import type { SceneExport, SceneProps } from './sceneTypes'",
            `export interface X { helper: (arg: SceneExport<import('${importPath}').SceneProps>) => SceneExport<SceneProps> | null }`,
            '',
        ].join('\n')

        writeFile(tsDir, 'sceneLogicType.ts', contents)
        writeFile(goDir, 'sceneLogicType.ts', contents)

        const summary = compareOutputs(tsDir, goDir)

        expect(summary.totalFiles).toBe(1)
        expect(summary.exactMatches).toBe(1)
        expect(summary.semanticMatches).toBe(1)
        expect(summary.semanticDiffs).toEqual([])
    })

    test('treats node_modules store paths and package import paths as semantically equal', () => {
        const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-compare-'))
        const tsDir = path.join(tempRoot, 'posthog-js', 'frontend', 'src', 'scenes')
        const goDir = path.join(tempRoot, 'posthog-go', 'frontend', 'src', 'scenes')
        const pnpmImportPath = path
            .join(
                tempRoot,
                'posthog',
                'node_modules',
                '.pnpm',
                'kea-router@3.4.1_kea@4.0.0-pre.5_react@18.3.1_',
                'node_modules',
                'kea-router',
                'lib',
                'types',
            )
            .replace(/\\/g, '/')

        writeFile(
            tsDir,
            'sceneLogicType.ts',
            [
                `import type { LocationChangedPayload } from '${path.relative(tsDir, pnpmImportPath).replace(/\\/g, '/')}'`,
                'export interface X { action: (payload: LocationChangedPayload) => void }',
                '',
            ].join('\n'),
        )
        writeFile(
            goDir,
            'sceneLogicType.ts',
            [
                "import type { LocationChangedPayload } from 'kea-router/lib/types'",
                'export interface X { action: (payload: LocationChangedPayload) => void }',
                '',
            ].join('\n'),
        )

        const summary = compareOutputs(tsDir, goDir)

        expect(summary.totalFiles).toBe(1)
        expect(summary.exactMatches).toBe(0)
        expect(summary.semanticMatches).toBe(1)
        expect(summary.semanticDiffs).toEqual([])
    })

    test('treats absolute local import-type paths and imported local names as semantically equal', () => {
        const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-compare-'))
        const tsDir = path.join(tempRoot, 'posthog-js', 'frontend', 'src', 'scenes')
        const goDir = path.join(tempRoot, 'posthog-go', 'frontend', 'src', 'scenes')
        const originalImportPath = path
            .join(tempRoot, 'original-posthog-js', 'frontend', 'src', 'scenes', 'sceneTypes')
            .replace(/\\/g, '/')

        writeFile(
            tsDir,
            'sceneLogicType.ts',
            [
                "import type { SceneExport, SceneProps } from './sceneTypes'",
                `export interface X { helper: (arg: SceneExport<import('${originalImportPath}').SceneProps>) => SceneExport<SceneProps> | null }`,
                '',
            ].join('\n'),
        )
        writeFile(
            goDir,
            'sceneLogicType.ts',
            [
                "import type { SceneExport, SceneProps } from './sceneTypes'",
                'export interface X { helper: (arg: SceneExport<SceneProps>) => SceneExport<SceneProps> | null }',
                '',
            ].join('\n'),
        )

        const summary = compareOutputs(tsDir, goDir)

        expect(summary.totalFiles).toBe(1)
        expect(summary.exactMatches).toBe(0)
        expect(summary.semanticMatches).toBe(1)
        expect(summary.semanticDiffs).toEqual([])
    })

    test('can limit comparison to selected files', () => {
        const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-compare-'))
        const tsDir = path.join(tempRoot, 'ts')
        const goDir = path.join(tempRoot, 'go')

        writeFile(tsDir, 'alphaType.ts', 'export interface Alpha { value: string }\n')
        writeFile(goDir, 'alphaType.ts', 'export interface Alpha { value: number }\n')
        writeFile(tsDir, 'betaType.ts', 'export interface Beta { value: string }\n')
        writeFile(goDir, 'betaType.ts', 'export interface Beta { value: string }\n')

        const summary = compareOutputs(tsDir, goDir, { files: ['betaType.ts'] })

        expect(summary.totalFiles).toBe(1)
        expect(summary.comparableFiles).toBe(1)
        expect(summary.exactMatches).toBe(1)
        expect(summary.semanticMatches).toBe(1)
        expect(summary.semanticDiffs).toEqual([])
    })
})
