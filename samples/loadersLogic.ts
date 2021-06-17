import { kea } from 'kea'
import { loadersLogicType } from './loadersLogicType'

import { Dashboard } from './types'

export const loadersLogic = kea<loadersLogicType>({
    actions: {
        addDashboard: (name: string) => ({ name }),
    },
    loaders: {
        dashboard: {
            __default: null as Dashboard | null,
            addDashboard: ({ name }: { name: string }): Dashboard => ({ id: -1, name, pinned: true }),
            addDashboardNoType: ({ name }: { name: string }): Dashboard => ({ id: -1, name, pinned: true }),
        },
        shouldNotBeNeverButAny: {
            __default: [],
        },
        misc: [
            {} as Record<string, any>,
            {
                loadIt: () => ({ id: -1, name, pinned: true }),
            },
        ],
    },
    reducers: () => ({
        dashboard: {
            addDashboardSuccess: (state, { dashboard }) => dashboard,
        },
    }),
})
