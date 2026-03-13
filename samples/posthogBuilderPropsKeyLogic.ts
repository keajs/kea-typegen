import { kea, key, path, props, reducers, selectors } from 'kea'
import type { posthogBuilderPropsKeyLogicType } from './posthogBuilderPropsKeyLogicType'

export type PosthogBuilderPropsKeyLogicProps = {
    resource: 'project' | 'feature_flag'
    resourceId: string
}

export const posthogBuilderPropsKeyLogic = kea<posthogBuilderPropsKeyLogicType>([
    props({} as PosthogBuilderPropsKeyLogicProps),
    key((props) => `${props.resource}-${props.resourceId}`),
    path((key) => ['posthogBuilderPropsKeyLogic', key]),
    reducers({}),
    selectors({
        endpoint: [
            (_, props) => [props.resource, props.resourceId],
            (resource, resourceId): string =>
                resource === 'project'
                    ? 'api/projects/@current/access_controls'
                    : `api/projects/@current/${resource}s/${resourceId}/access_controls`,
        ],
        humanReadableResource: [(_, props) => [props.resource], (resource) => resource.replace(/_/g, ' ')],
    }),
])
