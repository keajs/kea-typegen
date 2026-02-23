import { sourceToSourceFile, programFromSource, logicSourceToLogicType, gatherImports } from '../utils'
import * as fs from 'fs'
import * as os from 'os'
import * as path from 'path'
import * as ts from 'typescript'
import { ParsedLogic } from '../types'
import { SyntaxKind } from 'typescript'

test('sourceToSourceFile', () => {
    const source = 'var foo = 5;'
    const sourceFile1 = sourceToSourceFile(source)
    const sourceFile2 = sourceToSourceFile(source, 'otherLogic.ts')
    expect(sourceFile1.text).toEqual(source)
    expect(sourceFile2.text).toEqual(source)
    expect(sourceFile1.fileName).toEqual('logic.ts')
    expect(sourceFile2.fileName).toEqual('otherLogic.ts')
    expect(SyntaxKind[sourceFile1.kind]).toEqual('SourceFile')
    expect(SyntaxKind[sourceFile2.kind]).toEqual('SourceFile')
})

test('programFromSource', () => {
    const source = 'var foo = 5;'
    const program = programFromSource(source)
    expect(program).toBeDefined
    expect(typeof program.getSourceFile).toBe('function')
})

test('logicSourceToLogicType', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const myRandomLogic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            })
        })
    `
    const string = logicSourceToLogicType(logicSource)

    expect(string).toContain('export interface myRandomLogicType extends Logic {')
})

test('gatherImports prefers source package path for re-exported npm types', () => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'kea-typegen-imports-'))
    try {
        const logicPath = path.join(tempDir, 'logic.ts')
        const keaPath = path.join(tempDir, 'node_modules', 'kea', 'index.d.ts')
        const reactGridLayoutIndexPath = path.join(tempDir, 'node_modules', 'react-grid-layout', 'index.d.ts')
        const reactGridLayoutTypesPath = path.join(
            tempDir,
            'node_modules',
            'react-grid-layout',
            'dist',
            'types-BiXsdXr7.d.ts',
        )

        fs.mkdirSync(path.dirname(keaPath), { recursive: true })
        fs.mkdirSync(path.dirname(reactGridLayoutTypesPath), { recursive: true })

        fs.writeFileSync(
            logicPath,
            [
                "import { kea } from 'kea'",
                "import type { Layout, LayoutItem } from 'react-grid-layout'",
                '',
                'export const dashboardLogic = kea({',
                '    reducers: () => ({',
                '        currentLayout: [null as Layout | null, {}],',
                '        responsiveLayouts: [null as Record<string, LayoutItem[]> | null, {}],',
                '    }),',
                '})',
            ].join('\n'),
        )
        fs.writeFileSync(keaPath, 'export function kea(input: any): any')
        fs.writeFileSync(
            reactGridLayoutIndexPath,
            "export { L as Layout, a as LayoutItem } from './dist/types-BiXsdXr7'\n",
        )
        fs.writeFileSync(
            reactGridLayoutTypesPath,
            ['export type L = { i: string }', 'export type a = { x: number }'].join('\n'),
        )

        const program = ts.createProgram([logicPath], {
            module: ts.ModuleKind.CommonJS,
            moduleResolution: ts.ModuleResolutionKind.NodeJs,
            target: ts.ScriptTarget.ES2020,
            skipLibCheck: true,
        })
        const sourceFile = program.getSourceFile(logicPath)
        if (!sourceFile) {
            throw new Error('Expected test source file to exist')
        }

        const parsedLogic = {
            node: sourceFile,
            typeReferencesToImportFromFiles: {},
        } as unknown as ParsedLogic

        gatherImports(sourceFile, program.getTypeChecker(), parsedLogic)

        expect(parsedLogic.typeReferencesToImportFromFiles['react-grid-layout']).toEqual(
            new Set(['Layout', 'LayoutItem']),
        )
        expect(
            Object.keys(parsedLogic.typeReferencesToImportFromFiles).some((importPath) =>
                importPath.includes('react-grid-layout/dist/types-BiXsdXr7'),
            ),
        ).toBe(false)
    } finally {
        fs.rmSync(tempDir, { recursive: true, force: true })
    }
})
