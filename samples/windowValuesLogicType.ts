// Auto-generated with kea-typegen. DO NOT EDIT!

import { Logic } from 'kea'

export interface windowValuesLogicType extends Logic {
    actionCreators: {}
    actionKeys: {}
    actionTypes: {}
    actions: {}
    constants: {}
    defaults: {
        windowHeight: number
        windowWidth: number
    }
    events: {}
    key: undefined
    listeners: {}
    path: ['samples', 'windowValuesLogic']
    pathString: 'samples.windowValuesLogic'
    props: Record<string, unknown>
    reducer: (
        state: any,
        action: () => any,
        fullState: any,
    ) => {
        windowHeight: number
        windowWidth: number
    }
    reducerOptions: {}
    reducers: {
        windowHeight: (state: number, action: any, fullState: any) => number
        windowWidth: (state: number, action: any, fullState: any) => number
    }
    selector: (
        state: any,
    ) => {
        windowHeight: number
        windowWidth: number
    }
    selectors: {
        windowHeight: (state: any, props?: any) => number
        windowWidth: (state: any, props?: any) => number
    }
    sharedListeners: {}
    values: {
        windowHeight: number
        windowWidth: number
    }
    _isKea: true
    _isKeaWithKey: false
}
