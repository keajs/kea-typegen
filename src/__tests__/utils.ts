import { sourceToSourceFile, programFromSource, logicSourceToLogicType } from '../utils'
import * as ts from 'typescript'

test('sourceToSourceFile', () => {
    const source = 'var foo = 5;'
    const sourceFile1 = sourceToSourceFile(source, )
    const sourceFile2 = sourceToSourceFile(source, 'otherLogic.ts')
    expect(sourceFile1.text).toEqual(source)
    expect(sourceFile2.text).toEqual(source)
    expect(sourceFile1.fileName).toEqual('logic.ts')
    expect(sourceFile2.fileName).toEqual('otherLogic.ts')
    expect(ts.SyntaxKind[sourceFile1.kind]).toEqual('SourceFile')
    expect(ts.SyntaxKind[sourceFile2.kind]).toEqual('SourceFile')
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