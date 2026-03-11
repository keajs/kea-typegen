import { kea } from 'kea'

import * as github from './githubLogic'

export const githubNamespaceConnectLogic = kea({
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
