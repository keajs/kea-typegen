import { kea } from 'kea'

const logic = kea({
  path: () => ['scenes', 'homepage', 'index'],
  constants: () => ['SOMETHING', 'SOMETHING_ELSE'],
  actions: () => ({
    updateName: (name) => ({ name }),
  }),
  reducers: ({ actions }) => ({
    name: [
      'chirpy',
      {
        [actions.updateName]: (state, payload) => payload.name,
      },
    ],
  }),
  selectors: ({ selectors }) => ({
    upperCaseName: [
      () => [selectors.capitalizedName],
      (capitalizedName) => {
        return capitalizedName.toUpperCase()
      },
    ],
    capitalizedName: [
      () => [selectors.name],
      (name) => {
        return name
          .trim()
          .split(' ')
          .map((k) => `${k.charAt(0).toUpperCase()}${k.slice(1).toLowerCase()}`)
          .join(' ')
      },
    ],
  }),
  extend: [
    {
      constants: () => ['SOMETHING_BLUE', 'SOMETHING_ELSE'],
      actions: () => ({
        updateDescription: (description) => ({ description }),
      }),
      reducers: ({ actions }) => ({
        description: [
          '',
          {
            [actions.updateDescription]: (state, payload) => payload.description,
          },
        ],
      }),
      selectors: ({ selectors }) => ({
        upperCaseDescription: [() => [selectors.description], (description) => description.toUpperCase()],
      }),
    },
  ],
})
