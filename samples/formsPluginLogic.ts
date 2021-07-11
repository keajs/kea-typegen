import { kea, resetContext } from 'kea'
import { formsPlugin } from 'forms-plugin'
import { inlinePlugin } from './inline-plugin'
import { formsPluginLogicType } from './formsPluginLogicType'

export const formsPluginLogic = kea<formsPluginLogicType>({
    forms: { bla: true },

    inline: {
        default: {
            name: 'john',
            age: 123,
        },
    },
})

resetContext({
    plugins: [formsPlugin(), formsPlugin, inlinePlugin],
})
