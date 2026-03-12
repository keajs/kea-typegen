import { logicSourceToLogicType } from '../../utils'
import type { logicType, logicType, logicType, logicType } from './actionsType'

test('actions - with a function', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea<logicType>({
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
        
        const logic = kea<logicType>({
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
        
        const logic = kea<logicType>({
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
        
        const logic = kea<logicType>({
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
