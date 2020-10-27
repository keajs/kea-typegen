// Auto-generated with kea-typegen. DO NOT EDIT!

import { Logic } from 'kea'

export interface propsLogicType extends Logic {
    actionCreators: {
        setPage: (
            page: string,
        ) => {
            type: 'set page (samples.propsLogic)'
            payload: {
                page: string
            }
        }
        setId: (
            id: number,
        ) => {
            type: 'set id (samples.propsLogic)'
            payload: {
                id: number
            }
        }
    }
    actionKeys: {
        'set page (samples.propsLogic)': 'setPage'
        'set id (samples.propsLogic)': 'setId'
    }
    actionTypes: {
        setPage: 'set page (samples.propsLogic)'
        setId: 'set id (samples.propsLogic)'
    }
    actions: {
        setPage: (page: string) => void
        setId: (id: number) => void
    }
    constants: {}
    defaults: {
        currentPage: string
    }
    events: {}
    key: number
    listeners: {}
    path: ['samples', 'propsLogic']
    pathString: 'samples.propsLogic'
    props: {
        page: string
        id: number
    }
    reducer: (
        state: any,
        action: () => any,
        fullState: any,
    ) => {
        currentPage: string
    }
    reducerOptions: {}
    reducers: {
        currentPage: (state: string, action: any, fullState: any) => string
    }
    selector: (
        state: any,
    ) => {
        currentPage: string
    }
    selectors: {
        currentPage: (state: any, props?: any) => string
    }
    sharedListeners: {}
    values: {
        currentPage: string
    }
    _isKea: true
    _isKeaWithKey: true
}
