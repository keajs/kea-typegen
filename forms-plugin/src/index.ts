import { KeaPlugin } from 'kea'

export const formsPlugin = (): KeaPlugin => {
    return {
        name: 'forms',

        defaults: () => ({
            forms: undefined,
        }),

        buildSteps: {
            forms(logic, input) {
                if (!input.forms) {
                    return
                }

                logic.extend({ actions: { submitForm: true }, reducers: { form: [{ asd: true }] } })
            },
        },

        buildOrder: {
            forms: { after: 'defaults' },
        },
    }
}
