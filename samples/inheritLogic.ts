import { kea } from 'kea'
import { baseLogicType, inheritingLogicType } from './inheritLogicType'

export type Repository = {
    id: number
    stargazers_count: number
    html_url: string
    full_name: string
    forks: number
}

export const baseLogic = kea<baseLogicType<Repository>>({
    actions: {
        setUsername: (username: string) => ({ username }),
        setOtherUsername: (username: string) => ({ username }),
    },

    reducers: {
        username: [
            'keajs',
            {
                setUsername: (_, { username }) => username,
            },
        ],
    },
})

export const inheritingLogic = kea<inheritingLogicType>({
    inherit: [baseLogic],

    actions: {
        setOtherUsername: (username: number) => ({ username: username.toString() }),
        setFinalName: (username: string) => ({ username }),
    },

    reducers: {
        username: [
            'keajs',
            {
                setUsername: (_, { username }) => username,
            },
        ],
    },
})
