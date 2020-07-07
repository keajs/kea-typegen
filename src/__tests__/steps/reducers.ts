import { logicSourceToLogicType } from '../../index'

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
    reducer: (state: any, action: () => any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
    reducers: {
        name: (state: string, action: any) => string;
        otherNameNoDefault: (state: any, action: any) => any;
        yetAnotherNameWithNullDefault: (state: string | null, action: any) => string | null;
    };
    selectors: {
        name: (state: any, props: Record<string, any>) => string;
        otherNameNoDefault: (state: any, props: Record<string, any>) => any;
        yetAnotherNameWithNullDefault: (state: any, props: Record<string, any>) => string | null;
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
    reducer: (state: any, action: () => any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
    reducers: {
        name: (state: string, action: any) => string;
        otherNameNoDefault: (state: any, action: any) => any;
        yetAnotherNameWithNullDefault: (state: string | null, action: any) => string | null;
    };
    selectors: {
        name: (state: any, props: Record<string, any>) => string;
        otherNameNoDefault: (state: any, props: Record<string, any>) => any;
        yetAnotherNameWithNullDefault: (state: any, props: Record<string, any>) => string | null;
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
    reducer: (state: any, action: () => any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
    reducers: {
        name: (state: string, action: any) => string;
        otherNameNoDefault: (state: any, action: any) => any;
        yetAnotherNameWithNullDefault: (state: string | null, action: any) => string | null;
    };
    selectors: {
        name: (state: any, props: Record<string, any>) => string;
        otherNameNoDefault: (state: any, props: Record<string, any>) => any;
        yetAnotherNameWithNullDefault: (state: any, props: Record<string, any>) => string | null;
    };
    values: {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
    };
}`.trim(),
    )
})
