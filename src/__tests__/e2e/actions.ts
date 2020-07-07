import { logicSourceToLogicType } from '../../utils'

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
    reducer: (state: any, action: () => any, fullState: any) => {};
    reducers: {};
    selector: (state: any) => {};
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
    reducer: (state: any, action: () => any, fullState: any) => {};
    reducers: {};
    selector: (state: any) => {};
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
    reducer: (state: any, action: () => any, fullState: any) => {};
    reducers: {};
    selector: (state: any) => {};
    selectors: {};
    values: {};
}`.trim(),
    )
})

test('actions - with random values instead of functions', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: {
                updateName: (name: string) => ({ name }),
                withDefaultValue: (name = "john") => ({ name }),
                withDefaultValueAndType: (name: string = "john") => ({ name }),
                withBool: true,
                withRandomPayload: { bla: 123 },
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
        withDefaultValue: (name: any) => ({
            type: string;
            payload: { name: string; };
        });
        withDefaultValueAndType: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        withBool: () => ({
            type: string;
            payload: {
                value: boolean;
            };
        });
        withRandomPayload: () => ({
            type: string;
            payload: {
                value: { bla: number; };
            };
        });
    };
    actions: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        withDefaultValue: (name: any) => ({
            type: string;
            payload: { name: string; };
        });
        withDefaultValueAndType: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        withBool: () => ({
            type: string;
            payload: {
                value: boolean;
            };
        });
        withRandomPayload: () => ({
            type: string;
            payload: {
                value: { bla: number; };
            };
        });
    };
    reducer: (state: any, action: () => any, fullState: any) => {};
    reducers: {};
    selector: (state: any) => {};
    selectors: {};
    values: {};
}`.trim(),
    )
})
