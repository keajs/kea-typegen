// Auto-generated with kea-typegen. DO NOT EDIT!

import { Logic } from 'kea'

export interface githubImportLogicType<Repository> extends Logic {
    actionCreators: {}
    actionKeys: {}
    actionTypes: {}
    actions: {}
    constants: {}
    defaults: {
        repositoryReducerCopy: Repository[]
    }
    events: {}
    key: undefined
    listeners: {
        'set username (samples.githubLogic)': ((
            action: {
                type: 'set username (samples.githubLogic)'
                payload: {
                    username: string
                }
            },
            previousState: any,
        ) => void | Promise<void>)[]
    }
    path: ['samples', 'githubImportLogic']
    pathString: 'samples.githubImportLogic'
    props: Record<string, unknown>
    reducer: (
        state: any,
        action: () => any,
        fullState: any,
    ) => {
        repositoryReducerCopy: Repository[]
    }
    reducerOptions: {}
    reducers: {
        repositoryReducerCopy: (state: Repository[], action: any, fullState: any) => Repository[]
    }
    selector: (
        state: any,
    ) => {
        repositoryReducerCopy: Repository[]
    }
    selectors: {
        repositoryReducerCopy: (state: any, props?: any) => Repository[]
        repositorySelectorCopy: (state: any, props?: any) => Repository[]
    }
    sharedListeners: {}
    values: {
        repositoryReducerCopy: Repository[]
        repositorySelectorCopy: Repository[]
    }
    _isKea: true
    _isKeaWithKey: false
    __keaTypeGenInternalSelectorTypes: {
        repositorySelectorCopy: (repositories: Repository[]) => Repository[]
    }
    __keaTypeGenInternalReducerActions: {
        'set repositories (samples.githubLogic)': (
            repositories: Repository[],
        ) => {
            type: 'set repositories (samples.githubLogic)'
            payload: {
                repositories: Repository[]
            }
        }
        'set username (samples.githubLogic)': (
            username: string,
        ) => {
            type: 'set username (samples.githubLogic)'
            payload: {
                username: string
            }
        }
    }
}
