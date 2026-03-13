import { connect, kea, path, selectors } from 'kea'
import type { posthogMapConnectLogicType } from './posthogMapConnectLogicType'
import type { GroupType } from './posthogCanaryTypes'
import { posthogMapLogic } from './posthogMapLogic'

export const posthogMapConnectLogic = kea<posthogMapConnectLogicType>([
    path(['posthogMapConnectLogic']),
    connect(() => ({
        values: [posthogMapLogic, ['groupTypes']],
    })),
    selectors({
        groupNames: [
            (s) => [s.groupTypes],
            (groupTypes: Map<number, GroupType>): string[] => Array.from(groupTypes.values()).map((groupType) => groupType.name),
        ],
    }),
])
