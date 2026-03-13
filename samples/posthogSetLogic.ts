import { kea } from 'kea'
import type { posthogSetLogicType } from './posthogSetLogicType'
import type { ExperimentsResult, FeatureFlagsResult } from './posthogCanaryTypes'

export const posthogSetLogic = kea<posthogSetLogicType>({
    path: ['posthogSetLogic'],

    defaults: {
        featureFlags: {} as FeatureFlagsResult,
        experiments: {} as ExperimentsResult,
    },

    selectors: {
        unavailableFeatureFlagKeys: [
            (s) => [s.featureFlags, s.experiments],
            (featureFlags, experiments): Set<string> => {
                return new Set([
                    ...featureFlags.results.map((flag) => flag.key),
                    ...experiments.results.map((experiment) => experiment.feature_flag_key),
                ])
            },
        ],
    },
})
