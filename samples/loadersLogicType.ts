// Auto-generated with kea-typegen. DO NOT EDIT!

import { Logic } from 'kea'

export interface loadersLogicType<Dashboard> extends Logic {
    actionCreators: {
        addDashboard: (
            name: string,
        ) => {
            type: 'add dashboard (samples.loadersLogic)'
            payload: {
                name: string
            }
        }
        addDashboardSuccess: (
            dashboard: Dashboard,
        ) => {
            type: 'add dashboard success (samples.loadersLogic)'
            payload: {
                dashboard: Dashboard
            }
        }
        addDashboardFailure: (
            error: string,
        ) => {
            type: 'add dashboard failure (samples.loadersLogic)'
            payload: {
                error: string
            }
        }
        addDashboardNoType: ({
            name,
        }: {
            name: string
        }) => {
            type: 'add dashboard no type (samples.loadersLogic)'
            payload: {
                name: string
            }
        }
        addDashboardNoTypeSuccess: (
            dashboard: Dashboard,
        ) => {
            type: 'add dashboard no type success (samples.loadersLogic)'
            payload: {
                dashboard: Dashboard
            }
        }
        addDashboardNoTypeFailure: (
            error: string,
        ) => {
            type: 'add dashboard no type failure (samples.loadersLogic)'
            payload: {
                error: string
            }
        }
        loadIt: () => {
            type: 'load it (samples.loadersLogic)'
            payload: any
        }
        loadItSuccess: (
            misc: Record<string, any>,
        ) => {
            type: 'load it success (samples.loadersLogic)'
            payload: {
                misc: Record<string, any>
            }
        }
        loadItFailure: (
            error: string,
        ) => {
            type: 'load it failure (samples.loadersLogic)'
            payload: {
                error: string
            }
        }
    }
    actionKeys: {
        'add dashboard (samples.loadersLogic)': 'addDashboard'
        'add dashboard success (samples.loadersLogic)': 'addDashboardSuccess'
        'add dashboard failure (samples.loadersLogic)': 'addDashboardFailure'
        'add dashboard no type (samples.loadersLogic)': 'addDashboardNoType'
        'add dashboard no type success (samples.loadersLogic)': 'addDashboardNoTypeSuccess'
        'add dashboard no type failure (samples.loadersLogic)': 'addDashboardNoTypeFailure'
        'load it (samples.loadersLogic)': 'loadIt'
        'load it success (samples.loadersLogic)': 'loadItSuccess'
        'load it failure (samples.loadersLogic)': 'loadItFailure'
    }
    actionTypes: {
        addDashboard: 'add dashboard (samples.loadersLogic)'
        addDashboardSuccess: 'add dashboard success (samples.loadersLogic)'
        addDashboardFailure: 'add dashboard failure (samples.loadersLogic)'
        addDashboardNoType: 'add dashboard no type (samples.loadersLogic)'
        addDashboardNoTypeSuccess: 'add dashboard no type success (samples.loadersLogic)'
        addDashboardNoTypeFailure: 'add dashboard no type failure (samples.loadersLogic)'
        loadIt: 'load it (samples.loadersLogic)'
        loadItSuccess: 'load it success (samples.loadersLogic)'
        loadItFailure: 'load it failure (samples.loadersLogic)'
    }
    actions: {
        addDashboard: (name: string) => void
        addDashboardSuccess: (dashboard: Dashboard) => void
        addDashboardFailure: (error: string) => void
        addDashboardNoType: ({ name }: { name: string }) => void
        addDashboardNoTypeSuccess: (dashboard: Dashboard) => void
        addDashboardNoTypeFailure: (error: string) => void
        loadIt: () => void
        loadItSuccess: (misc: Record<string, any>) => void
        loadItFailure: (error: string) => void
    }
    constants: {}
    defaults: {
        dashboard: Dashboard | null
        dashboardLoading: boolean
        shouldNotBeNeverButAny: any[]
        shouldNotBeNeverButAnyLoading: boolean
        misc: Record<string, any>
        miscLoading: boolean
    }
    events: {}
    key: undefined
    listeners: {}
    path: ['samples', 'loadersLogic']
    pathString: 'samples.loadersLogic'
    props: Record<string, unknown>
    reducer: (
        state: any,
        action: () => any,
        fullState: any,
    ) => {
        dashboard: Dashboard | null
        dashboardLoading: boolean
        shouldNotBeNeverButAny: any[]
        shouldNotBeNeverButAnyLoading: boolean
        misc: Record<string, any>
        miscLoading: boolean
    }
    reducerOptions: {}
    reducers: {
        dashboard: (state: Dashboard | null, action: any, fullState: any) => Dashboard | null
        dashboardLoading: (state: boolean, action: any, fullState: any) => boolean
        shouldNotBeNeverButAny: (state: any[], action: any, fullState: any) => any[]
        shouldNotBeNeverButAnyLoading: (state: boolean, action: any, fullState: any) => boolean
        misc: (state: Record<string, any>, action: any, fullState: any) => Record<string, any>
        miscLoading: (state: boolean, action: any, fullState: any) => boolean
    }
    selector: (
        state: any,
    ) => {
        dashboard: Dashboard | null
        dashboardLoading: boolean
        shouldNotBeNeverButAny: any[]
        shouldNotBeNeverButAnyLoading: boolean
        misc: Record<string, any>
        miscLoading: boolean
    }
    selectors: {
        dashboard: (state: any, props?: any) => Dashboard | null
        dashboardLoading: (state: any, props?: any) => boolean
        shouldNotBeNeverButAny: (state: any, props?: any) => any[]
        shouldNotBeNeverButAnyLoading: (state: any, props?: any) => boolean
        misc: (state: any, props?: any) => Record<string, any>
        miscLoading: (state: any, props?: any) => boolean
    }
    sharedListeners: {}
    values: {
        dashboard: Dashboard | null
        dashboardLoading: boolean
        shouldNotBeNeverButAny: any[]
        shouldNotBeNeverButAnyLoading: boolean
        misc: Record<string, any>
        miscLoading: boolean
    }
    _isKea: true
    _isKeaWithKey: false
}
