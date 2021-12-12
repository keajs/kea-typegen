import { kea } from 'kea'

import { githubLogic } from './githubLogic'
import { githubConnectLogicType } from './githubConnectLogicType'

export const githubConnectLogic = kea<githubConnectLogicType>({
    path: ['githubConnectLogic'],
    connect: {
        values: [githubLogic, ['repositories', 'this-one-does-not-exist'], githubLogic(), ['isLoading']],
        actions: [githubLogic, ['setRepositories', 'will-not-be-imported']],
    },
})
