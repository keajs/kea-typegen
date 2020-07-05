import { kea } from 'kea'
import { logicInterface } from './logic.d'

export const logic: logicInterface = kea<logicInterface>({
    path: () => ['scenes', 'homepage', 'index'],
    constants: () => ['SOMETHING', 'SOMETHING_ELSE'],
    actions: () => ({
        updateName: (name: string) => ({ name }),
        updateOtherName: (otherName: string) => ({ otherName }),
    }),
    reducers: ({ actions }: logicInterface) => {
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
        }
    },
    selectors: ({ selectors }: logicInterface) => ({
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
