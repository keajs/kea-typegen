import { Program } from 'typescript'
import { replaceProgram } from '../typegen'

test('replaceProgram clears the previous Program reference before allocating the next one', () => {
    let currentProgram: Program | undefined = {} as Program
    const nextProgram = {} as Program
    let sawClearedReference = false

    const returnedProgram = replaceProgram(
        () => {
            sawClearedReference = typeof currentProgram === 'undefined'
            return nextProgram
        },
        (program) => {
            currentProgram = program
        },
    )

    expect(sawClearedReference).toBe(true)
    expect(returnedProgram).toBe(nextProgram)
    expect(currentProgram).toBe(nextProgram)
})
