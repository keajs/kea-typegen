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
