// Auto-generated with kea-typegen. DO NOT EDIT!

import { Logic } from 'kea'

export interface githubConnectLogicType<Repository> extends Logic {
    actionCreators: {
        setRepositories: (
            repositories: Repository[],
        ) => {
            type: 'set repositories (samples.githubConnectLogic)'
            payload: {
                repositories: Repository[]
            }
        }
    }
    actionKeys: {
        'set repositories (samples.githubConnectLogic)': 'setRepositories'
    }
    actionTypes: {
        setRepositories: 'set repositories (samples.githubConnectLogic)'
    }
    actions: {
        setRepositories: (repositories: Repository[]) => void
    }
    constants: {}
    defaults: {}
    events: {}
    key: undefined
    listeners: {}
    path: ['samples', 'githubConnectLogic']
    pathString: 'samples.githubConnectLogic'
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
