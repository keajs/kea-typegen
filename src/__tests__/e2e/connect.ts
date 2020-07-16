import {logicSourceToLogicType} from "../../utils";

test('connect actions from another logic', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        export interface otherLogicType {
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
            reducer: (state: any, action: () => any, fullState: any) => {};
            reducers: {};
            selector: (state: any) => {};
            selectors: {};
            values: {};
        }
        const otherLogic = {} as otherLogicType;
        const logic = kea({
            connect: {
                actions: [
                    otherLogic, [
                        'updateName',  
                        'updateOtherName',  
                    ]
                ]
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})


test('connect actions from multiple other logics', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        export interface otherLogicType {
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
            reducer: (state: any, action: () => any, fullState: any) => {};
            reducers: {};
            selector: (state: any) => {};
            selectors: {};
            values: {};
        }
        const otherLogic = {} as otherLogicType;
        const logic = kea({
            connect: {
                actions: [
                    otherLogic, [
                        'updateName'
                    ],
                    otherLogic, [  
                        'updateOtherName',  
                    ]
                ]
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('connect actions and rename', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        export interface otherLogicType {
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
            reducer: (state: any, action: () => any, fullState: any) => {};
            reducers: {};
            selector: (state: any) => {};
            selectors: {};
            values: {};
        }
        const otherLogic = {} as otherLogicType;
        const logic = kea({
            connect: {
                actions: [
                    otherLogic, [
                        'updateName as firstAction'
                    ],
                    otherLogic, [  
                        'updateOtherName as secondAction',  
                    ]
                ]
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('connect values from another logic', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        export interface otherLogicType {
            actionCreators: {};
            actions: {};
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
        }
        const otherLogic = {} as otherLogicType;
        const logic = kea({
            connect: {
                values: [
                    otherLogic, [
                        'name',
                    ],  
                    otherLogic, [
                        'yetAnotherNameWithNullDefault',  
                    ]
                ]
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('connect props from another logic', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        export interface otherLogicType {
            actionCreators: {};
            actions: {};
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
        }
        const otherLogic = {} as otherLogicType;
        const logic = kea({
            connect: {
                props: [
                    otherLogic, [
                        'name',
                    ],
                    otherLogic, [
                        'yetAnotherNameWithNullDefault',  
                    ]
                ]
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('connect values and rename', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        export interface otherLogicType {
            actionCreators: {};
            actions: {};
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
        }
        const otherLogic = {} as otherLogicType;
        const logic = kea({
            connect: {
                values: [
                    otherLogic, [
                        'name as firstSelector',
                    ],  
                    otherLogic, [
                        'yetAnotherNameWithNullDefault as secondSelector',  
                    ]
                ]
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})
