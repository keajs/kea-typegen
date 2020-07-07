import { programFromSource } from '../../utils'
import { visitProgram } from '../visit'

test('visitProgram', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            })
        })
    `

    const program = programFromSource(logicSource)
    const parsedLogics = visitProgram(program)

    expect(parsedLogics.length).toBe(1)
    expect(parsedLogics[0].actions.length).toBe(2)
    expect(parsedLogics[0].actions.map((a) => a.name)).toEqual(['updateName', 'updateOtherName'])
})
