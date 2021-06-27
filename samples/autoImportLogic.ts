import { kea, KeaPlugin } from 'kea'

import { autoImportLogicType } from './autoImportLogicType'
import { A1, A2, A3, A4, A5, A7, D1, D3, D6, EventIndex, ExportedApi, R1, R6, S6 } from './autoImportTypes'
import { githubLogic } from './githubLogic'
import { loadersLogic } from './loadersLogic'

type L1 = 'haha'
type L2 = {
    bla: string
}
interface RandomAPI extends ExportedApi.RandomThing {}

export const autoImportLogic = kea<autoImportLogicType<L1, L2, RandomAPI>>({
    props: {} as {
        asd: D3
    },
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
        valueAction: [] as A7,
        miscActionWithType: (randomThing: ExportedApi.RandomThing): { randomThing: ExportedApi.RandomThing } => ({
            randomThing,
        }),
        miscActionWithoutType: (randomThing: ExportedApi.RandomThing) => ({ randomThing }),
        miscInterfacedAction: (randomApi: RandomAPI) => ({ randomApi }),
    },
    connect: {
        actions: [githubLogic, ['setRepositories']],
        values: [loadersLogic, ['dashboard']],
    },
    defaults: {
        bla: { bla: 'asd' } as D1,
        asd: {} as D6,
    },
    reducers: {
        rbla: [{ bla: 'asd' } as R1, {}],
        rasd: [{} as R6, {}],
        then: [null as null | ExportedApi.RandomThing, {}],
    },
    selectors: {
        sbla: [(s) => [(): S6 => ({})], (a) => a],
        eventIndex: [() => [], () => new EventIndex()],
        randomDetectedReturn: [() => [], () => ({} as ExportedApi.RandomThing)],
        randomSpecifiedReturn: [() => [], (): ExportedApi.RandomThing => ({} as ExportedApi.RandomThing)],
        randomInterfacedReturn: [() => [], () => ({} as RandomAPI)],
    },
})
