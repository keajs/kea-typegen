import { actions, events, kea, listeners, reducers, selectors, path } from 'kea'
import { Repository } from './types'
import { lazyLoaders } from "kea-loaders"
import type { builderLogicType } from './builderLogicType'

const API_URL = 'https://api.github.com'

export const builderLogic = kea<builderLogicType>([
    path(['builderLogic']),
    actions({
        setUsername: (username: string) => ({ username }),
        setRepositories: (repositories: Repository[]) => ({ repositories }),
        setFetchError: (error: string) => ({ error }),
    }),
    reducers({
        username: [
            'keajs',
            {
                setUsername: (_, { username }) => username,
            },
        ],
        repositories: [
            [] as Repository[],
            {
                setUsername: () => [],
                setRepositories: (_, { repositories }) => repositories,
            },
        ],
        isLoading: [
            false,
            {
                setUsername: () => true,
                setRepositories: () => false,
                setFetchError: () => false,
            },
        ],
        error: [
            null as string | null,
            {
                setUsername: () => null,
                setFetchError: (_, { error }) => error,
            },
        ],
    }),
    selectors({
        sortedRepositories: [
            (selectors) => [selectors.repositories],
            (repositories) => {
                return [...repositories].sort((a, b) => b.stargazers_count - a.stargazers_count)
            },
        ],
    }),
    listeners(({ actions }) => ({
        setUsername: async ({ username }, breakpoint) => {
            await breakpoint(300)

            const url = `${API_URL}/users/${username}/repos?per_page=250`

            // 👈 handle network errors
            let response
            try {
                response = await window.fetch(url)
            } catch (error) {
                actions.setFetchError(error.message)
                return // 👈 nothing to do after, so return
            }

            // break if action was dispatched again while we were fetching
            breakpoint()

            const json = await response.json()

            if (response.status === 200) {
                actions.setRepositories(json)
            } else {
                actions.setFetchError(json.message)
            }
        },
    })),
    events(({ actions, values }) => ({
        afterMount: () => {
            actions.setUsername(values.username)
        },
    })),
    lazyLoaders({
        lazyValue: ['', { initValue: () => {} }]
    })
])
