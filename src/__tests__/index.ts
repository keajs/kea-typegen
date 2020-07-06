import { logicSourceToLogicType } from '../index'

test('logicSourceToLogicType', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            })
        })
    `
    const string = logicSourceToLogicType(logicSource)

    expect(string).toEqual(
        `
export interface logicType {
    actions: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: string;
            payload: { otherName: string; };
        });
    };
    actionsCreators: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: string;
            payload: { otherName: string; };
        });
    };
}`.trim(),
    )
})

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
