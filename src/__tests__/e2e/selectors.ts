import { logicSourceToLogicType } from '../../utils'

test('selectors - with a function', () => {
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
                    },
                ],
                otherName: {
                    updateOtherName: (_, { name }) => name,
                },
            }),
            selectors: ({ selectors }) => ({
                upperName: [
                    () => [selectors.name],
                    (name): string => name.toUpperCase()
                ],
                combinedLength: [
                    () => [selectors.name, selectors.otherName],
                    (name, otherName): number => name.length + otherName.length
                ]
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('selectors - as an object', () => {
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
                    },
                ],
                otherName: {
                    updateOtherName: (_, { name }) => name,
                },
            }),
            selectors: {
                upperName: [
                    (s) => [s.name],
                    (name): string => name.toUpperCase()
                ],
                combinedLength: [
                    (s) => [s.name, s.otherName],
                    (name, otherName): number => name.length + otherName.length
                ]
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('selectors - as a function returning a object', () => {
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
                    },
                ],
                otherName: {
                    updateOtherName: (_, { name }) => name,
                },
            })
            selectors: function ({ selectors }) {
                return {
                    upperName: [
                        () => [selectors.name],
                        (name): string => name.toUpperCase()
                    ],
                    combinedLength: [
                        () => [selectors.name, selectors.otherName],
                        (name, otherName): number => name.length + otherName.length
                    ]
                }
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})
