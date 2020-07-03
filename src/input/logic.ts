import { kea } from 'kea'

const logic = kea({
    path: () => ['scenes', 'homepage', 'index'],
    constants: () => [
        'SOMETHING',
        'SOMETHING_ELSE'
    ],
    actions: ({ constants }) => ({
        updateName: name => ({ name })
    }),
    reducers: ({ actions, constants }) => ({
        name: ['chirpy', PropTypes.string, {
            [actions.updateName]: (state, payload) => payload.name
        }]
    }),
    selectors: ({ constants, selectors }) => ({
        upperCaseName: [
            () => [selectors.capitalizedName],
            (capitalizedName) => {
                return capitalizedName.toUpperCase()
            },
            PropTypes.string
        ],
        capitalizedName: [
            () => [selectors.name],
            (name) => {
                return name.trim().split(' ').map(k => `${k.charAt(0).toUpperCase()}${k.slice(1).toLowerCase()}`).join(' ')
            },
            PropTypes.string
        ]
    }),
    extend: [
        {
            constants: () => [
                'SOMETHING_BLUE',
                'SOMETHING_ELSE'
            ],
            actions: ({ constants }) => ({
                updateDescription: description => ({ description })
            }),
            reducers: ({ actions, constants }) => ({
                description: ['', PropTypes.string, {
                    [actions.updateDescription]: (state, payload) => payload.description
                }]
            }),
            selectors: ({ constants, selectors }) => ({
                upperCaseDescription: [
                    () => [selectors.description],
                    (description) => description.toUpperCase(),
                    PropTypes.string
                ]
            })
        }
    ]
})
