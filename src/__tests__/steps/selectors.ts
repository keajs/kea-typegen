import { logicSourceToLogicType } from '../../index'

test('selectors - with a function', () => {
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
                    },
                ],
                otherName: {
                    updateOtherName: (_, { name }) => name,
                },
            }),
            selectors: ({ selectors }) => ({
                upperName: [
                    () => [selectors.name],
                    (name): string => name.toUpperCase()
                ],
                combinedLength: [
                    () => [selectors.name, selectors.otherName],
                    (name, otherName): number => name.length + otherName.length
                ]
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
    reducer: (state: any, action: () => any, fullState: any) => {
        name: string;
        otherName: any;
    };
    reducers: {
        name: (state: string, action: any, fullState: any) => string;
        otherName: (state: any, action: any, fullState: any) => any;
    };
    selector: (state: any) => {
        name: string;
        otherName: any;
    };
    selectors: {
        name: (state: any, props: Record<string, any>) => string;
        otherName: (state: any, props: Record<string, any>) => any;
        upperName: (state: any, props: Record<string, any>) => string;
        combinedLength: (state: any, props: Record<string, any>) => number;
    };
    values: {
        name: string;
        otherName: any;
        upperName: string;
        combinedLength: number;
    };
}`.trim(),
    )
})

test('selectors - as an object', () => {
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
                    },
                ],
                otherName: {
                    updateOtherName: (_, { name }) => name,
                },
            }),
            selectors: {
                upperName: [
                    (s) => [s.name],
                    (name): string => name.toUpperCase()
                ],
                combinedLength: [
                    (s) => [s.name, s.otherName],
                    (name, otherName): number => name.length + otherName.length
                ]
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
    reducer: (state: any, action: () => any, fullState: any) => {
        name: string;
        otherName: any;
    };
    reducers: {
        name: (state: string, action: any, fullState: any) => string;
        otherName: (state: any, action: any, fullState: any) => any;
    };
    selector: (state: any) => {
        name: string;
        otherName: any;
    };
    selectors: {
        name: (state: any, props: Record<string, any>) => string;
        otherName: (state: any, props: Record<string, any>) => any;
        upperName: (state: any, props: Record<string, any>) => string;
        combinedLength: (state: any, props: Record<string, any>) => number;
    };
    values: {
        name: string;
        otherName: any;
        upperName: string;
        combinedLength: number;
    };
}`.trim(),
    )
})

test('selectors - as a function returning a object', () => {
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
                    },
                ],
                otherName: {
                    updateOtherName: (_, { name }) => name,
                },
            })
            selectors: function ({ selectors }) {
                return {
                    upperName: [
                        () => [selectors.name],
                        (name): string => name.toUpperCase()
                    ],
                    combinedLength: [
                        () => [selectors.name, selectors.otherName],
                        (name, otherName): number => name.length + otherName.length
                    ]
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
    reducer: (state: any, action: () => any, fullState: any) => {
        name: string;
        otherName: any;
    };
    reducers: {
        name: (state: string, action: any, fullState: any) => string;
        otherName: (state: any, action: any, fullState: any) => any;
    };
    selector: (state: any) => {
        name: string;
        otherName: any;
    };
    selectors: {
        name: (state: any, props: Record<string, any>) => string;
        otherName: (state: any, props: Record<string, any>) => any;
        upperName: (state: any, props: Record<string, any>) => string;
        combinedLength: (state: any, props: Record<string, any>) => number;
    };
    values: {
        name: string;
        otherName: any;
        upperName: string;
        combinedLength: number;
    };
}`.trim(),
    )
})
