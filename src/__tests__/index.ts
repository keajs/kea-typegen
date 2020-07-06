import { logicSourceToLogicType } from '../index'

test('logic type names', () => {
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

    expect(string).toContain('export interface myRandomLogicType {')
})