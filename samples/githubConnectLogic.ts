import { kea } from 'kea'

import { githubLogic, Repository } from './github'
import { githubConnectLogicType } from './githubConnectLogic.type'

export const githubConnectLogic = kea<githubConnectLogicType<Repository>>({
    connect: {
        values: [
            githubLogic, ['repositories']
        ],
        actions: [
            githubLogic, ['setRepositories']
        ]
    }
})
