// Generated by kea-typegen on Thu, 17 Jun 2021 22:39:11 GMT. DO NOT EDIT THIS FILE MANUALLY.

import { Logic } from 'kea'

import { Repository } from './types'

export interface githubConnectLogicType extends Logic {
    actionCreators: {
        setRepositories: (repositories: Repository[]) => {
            type: 'set repositories (githubConnectLogic)'
            payload: {
                repositories: Repository[]
            }
        }
    }
    actionKeys: {
        'set repositories (githubConnectLogic)': 'setRepositories'
    }
    actionTypes: {
        setRepositories: 'set repositories (githubConnectLogic)'
    }
    actions: {
        setRepositories: (repositories: Repository[]) => void
    }
    constants: {}
    defaults: {}
    events: {}
    key: undefined
    listeners: {}
    path: ['githubConnectLogic']
    pathString: 'githubConnectLogic'
    props: Record<string, unknown>
    reducer: (state: any, action: () => any, fullState: any) => {}
    reducerOptions: {}
    reducers: {}
    selector: (state: any) => {}
    selectors: {
        repositories: (state: any, props?: any) => Repository[]
        isLoading: (state: any, props?: any) => boolean
    }
    sharedListeners: {}
    values: {
        repositories: Repository[]
        isLoading: boolean
    }
    _isKea: true
    _isKeaWithKey: false
}
