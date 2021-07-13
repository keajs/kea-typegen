import { kea } from 'kea'
import { router } from 'kea-router'
import { routerConnectLogicType } from './routerConnectLogicType'

const routerConnectLogic = kea<routerConnectLogicType>({
    path: ['containers', 'pages', 'main'],
    connect: {
        // pulling in actions from `router`
        actions: [router, ['locationChanged']],
    },
    // ...
    listeners: {
        locationChanged: ({ ...props }) => {
            console.log(props)
        },
    },
})
