import { KeaPlugin } from 'kea'

export const inlinePlugin = (): KeaPlugin => {
    return {
        name: 'inline',

        defaults: () => ({
            inline: undefined,
        }),

        buildSteps: {
            inline(logic, input) {
                if (!input.inline) {
                    return
                }

                logic.extend({ actions: { ranFormsPlugin: true } })
            },
        },

        buildOrder: {
            inline: { after: 'defaults' },
        },
    }
}
