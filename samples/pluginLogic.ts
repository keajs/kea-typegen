import { kea, resetContext } from 'kea'
import { formsPlugin } from 'forms-plugin'
import { inlinePlugin } from './inline-plugin'
import { pluginLogicType, anotherPluginLogicType } from './pluginLogicType'

export const pluginLogic = kea<pluginLogicType>({
    inline: { bla: true },

    form: {
        default: {
            name: 'john',
            age: 123,
        },
        submit: (form) => {
            console.log(form.name)
            console.log(form.age)
        },
    },
})

export const anotherPluginLogic = kea<anotherPluginLogicType>({
    form: ({ values }) => ({
        default: {
            name: 'john',
            age: 123,
        },
        submit: (form) => {
            console.log(values.inlineReducer)
            console.log(form.name)
            console.log(form.age)
        },
    }),
})

resetContext({
    plugins: [formsPlugin(), formsPlugin, inlinePlugin],
})
