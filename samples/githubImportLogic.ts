import { kea } from 'kea'

import { githubLogic, Repository } from './githubLogic'
import { githubImportLogicType } from './githubImportLogicType'

export const githubImportLogic = kea<githubImportLogicType<Repository>>({
    reducers: () => ({
        repositoryReducerCopy: [
            [] as Repository[],
            {
                [githubLogic.actionTypes.setRepositories]: (_, { repositories }) => repositories,
            },
        ],
    }),
    selectors: {
        repositorySelectorCopy: [() => [githubLogic.selectors.repositories], (repositories) => repositories],
    },
    listeners: () => ({
        [githubLogic.actionTypes.setRepositories]: ({ repositories }) => {
            console.log(repositories)
        },
    })
})
