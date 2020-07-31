// Auto-generated with kea-typegen. DO NOT EDIT!

import { Logic } from 'kea'

export interface propsLogicType extends Logic {
    key: number
    actionCreators: {
        setPage: (
            page: string,
        ) => {
            type: 'set page (samples.propsLogic)'
            payload: { page: string }
        }
        setId: (
            id: number,
        ) => {
            type: 'set id (samples.propsLogic)'
            payload: { id: number }
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
    cache: Record<string, any>
    constants: {}
    defaults: {
        currentPage: string
    }
    events: any
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
    reducerOptions: any
    reducers: {
        currentPage: (state: string, action: any, fullState: any) => string
    }
    selector: (
        state: any,
    ) => {
        currentPage: string
    }
    selectors: {
        currentPage: (state: any, props: any) => string
    }
    values: {
        currentPage: string
    }
    _isKea: true
}