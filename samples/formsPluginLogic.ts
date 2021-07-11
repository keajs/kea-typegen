import { kea, resetContext } from 'kea'
import { formsPlugin } from 'forms-plugin'

import { formsPluginLogicType } from './formsPluginLogicType'

export const formsPluginLogic = kea<formsPluginLogicType>({
    forms: { bla: true },
})
resetContext({
    plugins: [formsPlugin(), formsPlugin],
})
