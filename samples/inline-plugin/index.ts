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

                logic.extend({
                    actions: { inlineAction: true },
                    reducers: { inlineReducer: [(input.inline as any).default] },
                })
            },
        },

        buildOrder: {
            inline: { after: 'defaults' },
        },
    }
}
