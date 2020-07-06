import { kea } from 'kea'
import { logicType } from './logic.d'

export const logic: logicType = kea<logicType>({
    path: () => ['scenes', 'homepage', 'index'],
    constants: () => ['SOMETHING', 'SOMETHING_ELSE'],
    actions: () => ({
        updateName: (name: string) => ({ name }),
        updateOtherName: (otherName: string) => ({ otherName }),
    }),
    reducers: ({ actions }: logicType) => {
        return {
            name: [
                'birdname',
                {
                    updateName: (_, { name }) => name,
                    [actions.updateOtherName as any]: (state, payload) => payload.name,
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
    },
    selectors: ({ selectors }: logicType) => ({
        upperCaseName: [
            () => [selectors.capitalizedName],
            (capitalizedName) => {
                return capitalizedName.toUpperCase()
            },
        ],
        capitalizedName: [
            (s) => [s.name],
            (name) => {
                return name
                    .trim()
                    .split(' ')
                    .map((k) => `${k.charAt(0).toUpperCase()}${k.slice(1).toLowerCase()}`)
                    .join(' ')
            },
        ],
    }),
})
