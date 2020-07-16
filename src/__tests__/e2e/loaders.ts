import { logicSourceToLogicType } from '../../utils'

test('loaders - with a function', () => {
    const logicSource = `
        import { kea } from 'kea'
        interface Session {
            user: number,
            type: string
        }
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
            }),
            loaders: ({ actions }) => ({
                sessions: {
                    __default: [] as Session[],
                    loadSessions: async (query: string): Promise<Session[]> => {
                        const response = { user: 3, type: 'bla' }
                        return [response]
                    },
                },
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})


test('loaders - with an array and default', () => {
    const logicSource = `
        import { kea } from 'kea'
        interface Session {
            user: number,
            type: string
        }
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
            }),
            loaders: ({ actions }) => ({
                sessions: [
                    [] as Session[],
                    {
                        loadSessions: async (query: string): Promise<Session[]> => {
                            const response = { user: 3, type: 'bla' }
                            return [response]
                        },
                    }
                ],
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('loaders - with no param', () => {
    const logicSource = `
        import { kea } from 'kea'
        const logic = kea({
            loaders: ({ actions }) => ({
                sessions: [
                    [] as string[],
                    {
                        loadSessions: () => []
                        loadResults: async () => {
                            return []
                        }
                    }
                ],
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})
