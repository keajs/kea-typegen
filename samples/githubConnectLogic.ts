import { kea } from 'kea'

import { githubLogic, Repository } from './githubLogic'
import { githubConnectLogicType } from './githubConnectLogicType'

export const githubConnectLogic = kea<githubConnectLogicType<Repository>>({
    connect: {
        values: [
            githubLogic, ['repositories', 'this-one-does-not-exist'],
            githubLogic(), ['isLoading'],
        ],
        actions: [
            githubLogic, ['setRepositories', 'will-not-be-imported']
        ]
    }
})
