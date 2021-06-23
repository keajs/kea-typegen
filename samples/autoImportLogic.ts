import { kea, KeaPlugin } from 'kea'

import { autoImportLogicType } from './autoImportLogicType'
import { A1, A2, A3, A4, A5 } from './autoImportTypes'
import { githubLogic } from './githubLogic'
import { loadersLogic } from './loadersLogic'

type L1 = 'haha'
type L2 = {
    bla: string
}

export const autoImportLogic = kea<autoImportLogicType<L1, L2>>({
    actions: {
        actionA1: (
            local1: L1,
            local2: L2,
            param1: A1,
            param2: A2,
            param3: A3,
            param4: A4,
            param5: Partial<A5>,
            keaPlugin: KeaPlugin,
            stringType: string,
        ) => ({
            local1,
            local2,
            param1,
            param2,
            param3,
            param4,
            param5,
            keaPlugin,
            stringType,
        }),
        complexAction: (element: HTMLElement, timeout: NodeJS.Timeout) => ({ element, timeout }),
        combinedA6Action: (filter: A5) => ({ a6: filter.a6, bla: filter.bla }),
    },
    connect: {
        actions: [githubLogic, ['setRepositories']],
        values: [loadersLogic, ['dashboard']],
    },
})
