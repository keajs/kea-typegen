import { kea, KeaPlugin } from 'kea'

import { autoImportLogicType } from './autoImportLogicType'
import { T1, T2, T3, T4, T5 } from './autoImportTypes'
import { githubLogic } from './githubLogic'
import { loadersLogic } from './loadersLogic'

type L1 = 'haha'
type L2 = {
    bla: string
}

export const autoImportLogic = kea<autoImportLogicType<L1, L2>>({
    actions: {
        actionT1: (
            local1: L1,
            local2: L2,
            param1: T1,
            param2: T2,
            param3: T3,
            param4: T4,
            param5: Partial<T5>,
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
        combinedT6Action: (filter: T5) => ({ t6: filter.t6, bla: filter.bla }),
    },
    connect: {
        actions: [githubLogic, ['setRepositories']],
        values: [loadersLogic, ['dashboard']],
    },
})
