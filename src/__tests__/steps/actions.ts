import { logicSourceToLogicType } from '../../index'

test('actions - with a function', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toEqual(
        `
export interface logicType {
    actionCreators: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: string;
            payload: { otherName: string; };
        });
    };
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
    reducers: {};
    reducer: (state: any, action: () => any) => {};
    selectors: {};
    values: {};
}`.trim(),
    )
})

test('actions - as an object', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: {
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toEqual(
        `
export interface logicType {
    actionCreators: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: string;
            payload: { otherName: string; };
        });
    };
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
    reducers: {};
    reducer: (state: any, action: () => any) => {};
    selectors: {};
    values: {};
}`.trim(),
    )
})

test('actions - as a function returning a object', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: function () {
                return {
                    updateName: (name: string) => ({ name }),
                    updateOtherName: (otherName: string) => ({ otherName }),
                }
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toEqual(
        `
export interface logicType {
    actionCreators: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: string;
            payload: { otherName: string; };
        });
    };
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
    reducers: {};
    reducer: (state: any, action: () => any) => {};
    selectors: {};
    values: {};
}`.trim(),
    )
})
