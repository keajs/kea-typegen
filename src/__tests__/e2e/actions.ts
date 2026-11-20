import { logicSourceToLogicType } from '../../utils'

test('actions - with a function', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('actions - as an object', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: {
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('actions - as a function returning a object', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: function () {
                return {
                    updateName: (name: string) => ({ name }),
                    updateOtherName: (otherName: string) => ({ otherName }),
                }
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('actions - with random values instead of functions', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: {
                updateName: (name?: string) => ({ name }),
                withDefaultValue: (name = "john") => ({ name }),
                withDefaultValueAndType: (name: string = "john") => ({ name }),
                withTrue: true,
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})

test('actions - with generic', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: {
                updateName: <T extends string>(name?: T) => ({ name }),
            }
        })
    `
    expect(logicSourceToLogicType(logicSource)).toMatchSnapshot()
})