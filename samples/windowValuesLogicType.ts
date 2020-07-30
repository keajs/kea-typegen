// Auto-generated with kea-typegen. DO NOT EDIT!

export interface windowValuesLogicType {
    key: undefined
    actionCreators: {}
    actionKeys: {}
    actionTypes: {}
    actions: {}
    cache: Record<string, any>
    connections: any
    constants: any
    defaults: {
        windowHeight: number
        windowWidth: number
    }
    events: any
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
    reducerOptions: any
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
        windowHeight: (state: any, props: any) => number
        windowWidth: (state: any, props: any) => number
    }
    values: {
        windowHeight: number
        windowWidth: number
    }
    _isKea: true
}
