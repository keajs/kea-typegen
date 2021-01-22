import { kea, useValues } from 'kea'
import { logicType } from './logicType'
import { githubLogic } from './githubLogic'

interface Session {
    user: number
    type: string
}

export const logic = kea<logicType<Session>>({
    props: {} as {
        id: number
    },
    key: (props) => props.id,
    path: (key) => ['scenes', 'homepage', 'index', key],
    constants: () => ['SOMETHING', 'SOMETHING_ELSE'],
    actions: () => ({
        updateName: (name: string) => ({ name }),
        updateConst: (name: 'John' | 'Bill') => ({ name }),
        updateNumber: (number: number) => ({ number }),
    }),
    defaults: {
        yetAnotherNameWithNullDefault: "blue" as string | null
    },
    reducers: () => {
        return {
            name: [
                'birdname',
                {
                    updateName: (_, { name }) => name,
                    [githubLogic.actionTypes.setUsername]: (_, { username }) => username + '',
                },
            ],
            number: [
                1232,
                {
                    updateNumber: (_, { number }) => number,
                },
            ],
            persistedNumber: [
                1232,
                { persist: true },
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
        capitalizedName: [
            (s) => [s.name, s.number],
            (name, number) => {
                return (
                    name
                        .trim()
                        .split(' ')
                        .map((k) => `${k.charAt(0).toUpperCase()}${k.slice(1).toLowerCase()}`)
                        .join(' ') + number.toString()
                )
            },
        ],
        upperCaseName: [
            () => [selectors.capitalizedName],
            (capitalizedName) => {
                return capitalizedName.toUpperCase()
            },
        ],
        randomSelector: [
            () => [selectors.capitalizedName],
            (capitalizedName) => ({
                name: capitalizedName
            }) as Record<string, any>,
        ],
        longSelector: [
            (s) => [s.name, s.number, s.capitalizedName, s.upperCaseName, s.randomSelector, s.randomSelector],
            (a1, a2, a3, a4, a5, a6) => false
        ]
    }),
    loaders: () => ({
        sessions: {
            __default: [] as Session[],
            loadSessions: async (selectedDate: string): Promise<Session[]> => {
                const response = { user: 3, type: 'bla' }
                return [response]
            },
        },
    }),
    listeners: ({ actions, sharedListeners }) => ({
        updateNumber: ({ number }) => {
            actions.updateName(number.toString())
            console.log(number)
        },
        updateName: sharedListeners.someRandomFunction
    }),
    sharedListeners: () => ({
        someRandomFunction: ({ name } : { name: string, id?: number }) => {
            console.log('haha')
        }
    }),
    events: { afterMount: () => {} },
})

function MyComponent() {
    const {} = useValues(logic)

}
