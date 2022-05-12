import { kea } from 'kea'
import { windowValuesLogicType } from './windowValuesLogicType'
export const windowValuesLogic = kea<windowValuesLogicType>({
    path: ['windowValuesLogic'],
    windowValues: () => ({
        windowHeight: (window) => window.innerHeight,
        windowWidth: (window) => Math.min(window.innerWidth, window.document.body.clientWidth),
    }),
    // some kea-saga as well, because why not
    start: function * () {
        console.log('haha')
    },
    stop: function * () {
        console.log('haha')
    },
    saga: function * () {
        console.log('haha')
    },
    workers: {
        a: () => {}
    }
})
