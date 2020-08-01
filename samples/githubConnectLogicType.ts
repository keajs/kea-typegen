// Auto-generated with kea-typegen. DO NOT EDIT!

import { Logic } from 'kea'

export interface githubConnectLogicType<Repository> extends Logic {
    key: undefined
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
    cache: Record<string, any>
    constants: {}
    defaults: {}
    events: any
    path: ['samples', 'githubConnectLogic']
    pathString: 'samples.githubConnectLogic'
    props: Record<string, unknown>
    reducer: (state: any, action: () => any, fullState: any) => {}
    reducerOptions: {}
    reducers: {}
    selector: (state: any) => {}
    selectors: {
        repositories: (state: any, props: any) => Repository[]
    }
    values: {
        repositories: Repository[]
    }
    _isKea: true
}
