import { kea } from '../kea/kea'
import { logicType } from './logic.type'

interface Session {
    user: number
    type: string
}

export const logic = kea<logicType<Session>>({
    path: () => ['scenes', 'homepage', 'index'],
    constants: () => ['SOMETHING', 'SOMETHING_ELSE'],
    actions: () => ({
        updateName: (name: string) => ({ name }),
        updateNumber: (number: number) => ({ number }),
    }),
    reducers: ({ actions }) => {
        return {
            name: [
                'birdname',
                {
                    updateName: (_, { name }) => name,
                    [actions.updateNumber as any]: (state, payload) => payload.name + '',
                },
            ],
            number: [
                1232,
                {
                    updateNumber: (_, { number }) => number,
                },
            ],
            otherNameNoDefault: {
                updateName: (_, { name }) => name,
            },
            yetAnotherNameWithNullDefault: [
                null as string | null,
                {
                    updateName: (_, { name }) => name,
                },
            ],
        }
    },
    selectors: ({ selectors, values }) => ({
        upperCaseName: [
            () => [selectors.capitalizedName],
            (capitalizedName) => {
                return capitalizedName.toUpperCase()
            },
        ],
        capitalizedName: [
            (s) => [s.name, s.number],
            (name, number) => {
                return (
                    name
                        .trim()
                        .split(' ')
                        .map((k) => `${k.charAt(0).toUpperCase()}${k.slice(1).toLowerCase()}`)
                        .join(' ') + number
                )
            },
        ],
    }),
    loaders: ({ actions }) => ({
        sessions: {
            __default: [] as Session[],
            loadSessions: async (selectedDate): Promise<Session[]> => {
                const response = { user: 3, type: 'bla' }
                return [response]
            },
        },
    }),
})
