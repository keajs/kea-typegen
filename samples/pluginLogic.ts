import { kea, resetContext } from 'kea'
import { formsPlugin } from 'forms-plugin'
import { inlinePlugin } from './inline-plugin'
import { pluginLogicType, anotherPluginLogicType } from './pluginLogicType'

export const pluginLogic = kea<pluginLogicType>({
    forms: { bla: true },

    inline: {
        default: {
            name: 'john',
            age: 123,
        },
    },
})

export const anotherPluginLogic = kea<anotherPluginLogicType>({
    inline: () => ({
        default: {
            name: 'john',
            age: 123,
        },
    }),
})

resetContext({
    plugins: [formsPlugin(), formsPlugin, inlinePlugin],
})
