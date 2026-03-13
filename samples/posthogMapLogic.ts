import { kea, path, reducers, selectors } from 'kea'
import type { posthogMapLogicType } from './posthogMapLogicType'
import type { GroupType } from './posthogCanaryTypes'

export const posthogMapLogic = kea<posthogMapLogicType>([
    path(['posthogMapLogic']),
    reducers({
        groupTypesRaw: [[] as GroupType[], {}],
    }),
    selectors({
        groupTypes: [
            (s) => [s.groupTypesRaw],
            (groupTypesRaw) =>
                new Map<number, GroupType>(
                    (groupTypesRaw ?? []).map((groupType) => [groupType.group_type_index, groupType]),
                ),
        ],
    }),
])
