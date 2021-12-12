import { kea, resetContext } from 'kea'
import { formPlugin } from 'form-plugin'
import { inlinePlugin } from './inline-plugin'
import { pluginLogicType, anotherPluginLogicType } from './pluginLogicType'

export const pluginLogic = kea<pluginLogicType>({
    path: ['pluginLogic'],
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
    path: ['pluginLogic'],
    form: ({ values }) => ({
        default: {
            name: 'john',
            age: 123,
        },
        submit: (form) => {
            console.log(values.form)
            console.log(form.name)
            console.log(form.age)
        },
    }),
})

resetContext({
    plugins: [formPlugin(), formPlugin, inlinePlugin],
})
