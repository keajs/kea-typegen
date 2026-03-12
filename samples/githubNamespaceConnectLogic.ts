import { kea } from 'kea'

import * as github from './githubLogic'

import type { githubNamespaceConnectLogicType } from './githubNamespaceConnectLogicType'

export const githubNamespaceConnectLogic = kea<githubNamespaceConnectLogicType>({
    path: ['githubNamespaceConnectLogic'],
    connect: {
        actions: [github.githubLogic, ['setRepositories']],
        values: [github.githubLogic(), ['repositories', 'isLoading as githubIsLoading']],
    },
    listeners: () => ({
        [github.githubLogic.actionTypes.setRepositories]: ({ repositories }) => {
            console.log(repositories)
        },
    }),
})
