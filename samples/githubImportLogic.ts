import { kea } from 'kea'

import { githubLogic } from './githubLogic'
import { githubImportLogicType } from './githubImportLogicType'
import { Repository } from './types'

export const githubImportLogic = kea<githubImportLogicType>({
    path: ['githubImportLogic'],
    reducers: () => ({
        repositoryReducerCopy: [
            [] as Repository[],
            {
                [githubLogic.actionTypes.setRepositories]: (_, { repositories }) => repositories,
                [githubLogic().actionTypes.setRepositories]: (_, { repositories }) => repositories,
            },
        ],
    }),
    selectors: {
        repositorySelectorCopy: [() => [githubLogic.selectors.repositories], (repositories) => repositories],
    },
    listeners: () => ({
        [githubLogic.actionTypes.setUsername]: ({ username }) => {
            console.log(username)
        },
    }),
    sharedListeners: ({}) => ({
        bla(_, breakpoint, asd) {},
    }),
})
