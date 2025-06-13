import { actions, events, kea, listeners, reducers, selectors, path } from 'kea'
import { Repository } from './types'
import { lazyLoaders } from "kea-loaders"
import { forms } from "kea-forms"
import type { builderLogicType } from './builderLogicType'

const API_URL = 'https://api.github.com'

export type DashboardItemType = {
    key1: string
    key2: string
    key3: string
    key4: string
    key5: string
    key6: string
    key7: string
    key8: string
    key9: string
    key10: string
    key11: string
    key12: string
    key13: string
    key14: string
    key15: string
    key16: string
    key17: string
    key18: string
    key19: string
    key20: string
    key21: string
    key22: string
    key23: string
    key24: string
    key25: string
    key26: string
    key27: string
    key28: string
    key29: string
    key30: string
    key31: string
    key32: string
    key33: string
    key34: string
    key35: string
    key36: string
    key37: string
    key38: string
    key39: string
}
const item = {} as any as DashboardItemType

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
        lazyValue: ['', { initValue: () => {} }],
    }),
    forms(() => ({
        myForm: {
            defaults: { ...item, asd: true },
            submit: (form) => {
                console.log('Form submitted:', form)
            },
        },
    })),
])
