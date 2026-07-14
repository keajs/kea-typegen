import { kea, MakeLogicType } from 'kea'

interface ManualValues {
    localValue: string
}

interface ManualActions {
    setLocalValue: (value: string) => { value: string }
}

// this logic is typed manually, so kea-typegen leaves it alone
export const manualTypedLogic = kea<MakeLogicType<ManualValues, ManualActions>>({
    path: ['manualTypedLogic'],
    actions: {
        setLocalValue: (value: string) => ({ value }),
    },
    reducers: {
        localValue: ['', { setLocalValue: (_, { value }) => value }],
    },
})
