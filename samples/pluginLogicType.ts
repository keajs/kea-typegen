// Generated by kea-typegen on Fri, 10 Jan 2025 11:14:06 GMT. DO NOT EDIT THIS FILE MANUALLY.

import type { Logic, NotReallyHereButImportedBecausePlugin } from 'kea'

export interface pluginLogicType extends Logic {
    actionCreators: {
        inlineAction: () => {
            type: 'inline action (pluginLogic)'
            payload: {
                value: boolean
            }
        }
        submitForm: () => {
            type: 'submit form (pluginLogic)'
            payload: {
                value: boolean
            }
        }
    }
    actionKeys: {
        'inline action (pluginLogic)': 'inlineAction'
        'submit form (pluginLogic)': 'submitForm'
    }
    actionTypes: {
        inlineAction: 'inline action (pluginLogic)'
        submitForm: 'submit form (pluginLogic)'
    }
    actions: {
        inlineAction: () => void
        submitForm: () => void
    }
    asyncActions: {
        inlineAction: () => Promise<any>
        submitForm: () => Promise<any>
    }
    defaults: {
        inlineReducer: {
            asd: boolean
        }
        form: {
            age: number
            name: string
        }
    }
    events: {}
    key: undefined
    listeners: {}
    path: ['pluginLogic']
    pathString: 'pluginLogic'
    props: Record<string, unknown>
    reducer: (
        state: any,
        action: any,
        fullState: any,
    ) => {
        inlineReducer: {
            asd: boolean
        }
        form: {
            age: number
            name: string
        }
    }
    reducers: {
        inlineReducer: (
            state: {
                asd: boolean
            },
            action: any,
            fullState: any,
        ) => {
            asd: boolean
        }
        form: (
            state: {
                age: number
                name: string
            },
            action: any,
            fullState: any,
        ) => {
            age: number
            name: string
        }
    }
    selector: (state: any) => {
        inlineReducer: {
            asd: boolean
        }
        form: {
            age: number
            name: string
        }
    }
    selectors: {
        inlineReducer: (
            state: any,
            props?: any,
        ) => {
            asd: boolean
        }
        form: (
            state: any,
            props?: any,
        ) => {
            age: number
            name: string
        }
    }
    sharedListeners: {}
    values: {
        inlineReducer: {
            asd: boolean
        }
        form: {
            age: number
            name: string
        }
    }
    _isKea: true
    _isKeaWithKey: false
    __keaTypeGenInternalExtraInput: {
        form:
            | {
                  default?: Record<string, any>
                  submit?: (form: { age: number; name: string }) => void
              }
            | ((logic: pluginLogicType) => {
                  default?: Record<string, any>
                  submit?: (form: { age: number; name: string }) => void
              })
    }
}


export interface anotherPluginLogicType extends Logic {
    actionCreators: {
        submitForm: () => {
            type: 'submit form (pluginLogic)'
            payload: {
                value: boolean
            }
        }
    }
    actionKeys: {
        'submit form (pluginLogic)': 'submitForm'
    }
    actionTypes: {
        submitForm: 'submit form (pluginLogic)'
    }
    actions: {
        submitForm: () => void
    }
    asyncActions: {
        submitForm: () => Promise<any>
    }
    defaults: {
        form: {
            age: number
            name: string
        }
    }
    events: {}
    key: undefined
    listeners: {}
    path: ['pluginLogic']
    pathString: 'pluginLogic'
    props: Record<string, unknown>
    reducer: (
        state: any,
        action: any,
        fullState: any,
    ) => {
        form: {
            age: number
            name: string
        }
    }
    reducers: {
        form: (
            state: {
                age: number
                name: string
            },
            action: any,
            fullState: any,
        ) => {
            age: number
            name: string
        }
    }
    selector: (state: any) => {
        form: {
            age: number
            name: string
        }
    }
    selectors: {
        form: (
            state: any,
            props?: any,
        ) => {
            age: number
            name: string
        }
    }
    sharedListeners: {}
    values: {
        form: {
            age: number
            name: string
        }
    }
    _isKea: true
    _isKeaWithKey: false
    __keaTypeGenInternalExtraInput: {
        form:
            | {
                  default?: Record<string, any>
                  submit?: (form: { age: number; name: string }) => void
              }
            | ((logic: anotherPluginLogicType) => {
                  default?: Record<string, any>
                  submit?: (form: { age: number; name: string }) => void
              })
    }
}
