import { KeaPlugin } from 'kea'

export const formPlugin = (): KeaPlugin => {
    return {
        name: 'form',

        defaults: () => ({
            form: undefined,
        }),

        buildSteps: {
            form(logic, input) {
                if (!input.form) {
                    return
                }

                logic.extend({ actions: { submitForm: true }, reducers: { form: [{ asd: true }] } })
            },
        },

        buildOrder: {
            form: { after: 'defaults' },
        },
    }
}
