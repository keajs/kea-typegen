import { logicSourceToLogicType } from '../../utils'

test('reducers - with a function', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            }),
            reducers: () => ({
                name: [
                    'my name',
                    {
                        updateName: (_, { name }) => name,
                        [actions.updateOtherName]: (state, payload) => payload.name,
                    },
                ],
                otherNameNoDefault: {
                    updateName: (_, { name }) => name,
                },
                yetAnotherNameWithNullDefault: [
                    null as (string | null),
                    {
                        updateName: (_, { name }) => name,
                    },
                ],
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toEqual(
        `
export interface logicType {
    actionCreators: {
        updateName: (name: string) => ({
            type: "update name (logic)";
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: "update other name (logic)";
            payload: { otherName: string; };
        });
    };
    actions: {
        updateName: (name: string) => ({
            type: "update name (logic)";
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: "update other name (logic)";
            payload: { otherName: string; };
        });
    };
    reducer: (state: any, action: () => any, fullState: any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
    reducers: {
        name: (state: string, action: any, fullState: any) => string;
        otherNameNoDefault: (state: any, action: any, fullState: any) => any;
        yetAnotherNameWithNullDefault: (state: string | null, action: any, fullState: any) => string | null;
    };
    selector: (state: any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
    selectors: {
        name: (state: any, props: any) => string;
        otherNameNoDefault: (state: any, props: any) => any;
        yetAnotherNameWithNullDefault: (state: any, props: any) => string | null;
    };
    values: {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
}`.trim(),
    )
})

test('reducers - as an object', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            }),
            reducers: {
                name: [
                    'my name',
                    {
                        updateName: (_, { name }) => name,
                    },
                ],
                otherNameNoDefault: {
                    updateName: (_, { name }) => name,
                },
                yetAnotherNameWithNullDefault: [
                    null as (string | null),
                    {
                        updateName: (_, { name }) => name,
                    },
                ],
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toEqual(
        `
export interface logicType {
    actionCreators: {
        updateName: (name: string) => ({
            type: "update name (logic)";
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: "update other name (logic)";
            payload: { otherName: string; };
        });
    };
    actions: {
        updateName: (name: string) => ({
            type: "update name (logic)";
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: "update other name (logic)";
            payload: { otherName: string; };
        });
    };
    reducer: (state: any, action: () => any, fullState: any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
    reducers: {
        name: (state: string, action: any, fullState: any) => string;
        otherNameNoDefault: (state: any, action: any, fullState: any) => any;
        yetAnotherNameWithNullDefault: (state: string | null, action: any, fullState: any) => string | null;
    };
    selector: (state: any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
    selectors: {
        name: (state: any, props: any) => string;
        otherNameNoDefault: (state: any, props: any) => any;
        yetAnotherNameWithNullDefault: (state: any, props: any) => string | null;
    };
    values: {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
}`.trim(),
    )
})

test('reducers - as a function returning a object', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            }),
            reducers: () => { 
                return {
                    name: [
                        'my name',
                        {
                            updateName: (_, { name }) => name,
                            [actions.updateOtherName]: (state, payload) => payload.name,
                        },
                    ],
                    otherNameNoDefault: {
                        updateName: (_, { name }) => name,
                    },
                    yetAnotherNameWithNullDefault: [
                        null as (string | null),
                        {
                            updateName: (_, { name }) => name,
                        },
                    ],
                }
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toEqual(
        `
export interface logicType {
    actionCreators: {
        updateName: (name: string) => ({
            type: "update name (logic)";
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: "update other name (logic)";
            payload: { otherName: string; };
        });
    };
    actions: {
        updateName: (name: string) => ({
            type: "update name (logic)";
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: "update other name (logic)";
            payload: { otherName: string; };
        });
    };
    reducer: (state: any, action: () => any, fullState: any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
    reducers: {
        name: (state: string, action: any, fullState: any) => string;
        otherNameNoDefault: (state: any, action: any, fullState: any) => any;
        yetAnotherNameWithNullDefault: (state: string | null, action: any, fullState: any) => string | null;
    };
    selector: (state: any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
    selectors: {
        name: (state: any, props: any) => string;
        otherNameNoDefault: (state: any, props: any) => any;
        yetAnotherNameWithNullDefault: (state: any, props: any) => string | null;
    };
    values: {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
}`.trim(),
    )
})
