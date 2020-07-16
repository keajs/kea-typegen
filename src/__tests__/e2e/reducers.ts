import { logicSourceToLogicType } from '../../utils'

test('reducers - with a function', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            }),
            reducers: () => ({
                name: [
                    'my name',
                    {
                        updateName: (_, { name }) => name,
                        [actions.updateOtherName]: (state, payload) => payload.name,
                    },
                ],
                otherNameNoDefault: {
                    updateName: (_, { name }) => name,
                },
                yetAnotherNameWithNullDefault: [
                    null as (string | null),
                    {
                        updateName: (_, { name }) => name,
                    },
                ],
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('reducers - as an object', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            }),
            reducers: {
                name: [
                    'my name',
                    {
                        updateName: (_, { name }) => name,
                    },
                ],
                otherNameNoDefault: {
                    updateName: (_, { name }) => name,
                },
                yetAnotherNameWithNullDefault: [
                    null as (string | null),
                    {
                        updateName: (_, { name }) => name,
                    },
                ],
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('reducers - as a function returning a object', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            }),
            reducers: () => { 
                return {
                    name: [
                        'my name',
                        {
                            updateName: (_, { name }) => name,
                            [actions.updateOtherName]: (state, payload) => payload.name,
                        },
                    ],
                    otherNameNoDefault: {
                        updateName: (_, { name }) => name,
                    },
                    yetAnotherNameWithNullDefault: [
                        null as (string | null),
                        {
                            updateName: (_, { name }) => name,
                        },
                    ],
                }
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('reducers - with bool default', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: true
            }),
            reducers: () => { 
                return {
                    name: [
                        false,
                        {
                            updateName: () => true,
                        },
                    ]
                }
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})
