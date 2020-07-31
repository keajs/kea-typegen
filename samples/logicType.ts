// Auto-generated with kea-typegen. DO NOT EDIT!

export interface logicType<Session> {
    key: number
    actionCreators: {
        updateName: (
            name: string,
        ) => {
            type: 'update name (scenes.homepage.index.*)'
            payload: { name: string }
        }
        updateNumber: (
            number: number,
        ) => {
            type: 'update number (scenes.homepage.index.*)'
            payload: { number: number }
        }
        loadSessions: (
            selectedDate: string,
        ) => {
            type: 'load sessions (scenes.homepage.index.*)'
            payload: string
        }
        loadSessionsSuccess: (
            sessions: Session[],
        ) => {
            type: 'load sessions success (scenes.homepage.index.*)'
            payload: {
                sessions: Session[]
            }
        }
        loadSessionsFailure: (
            error: string,
        ) => {
            type: 'load sessions failure (scenes.homepage.index.*)'
            payload: {
                error: string
            }
        }
    }
    actionKeys: {
        'update name (scenes.homepage.index.*)': 'updateName'
        'update number (scenes.homepage.index.*)': 'updateNumber'
        'load sessions (scenes.homepage.index.*)': 'loadSessions'
        'load sessions success (scenes.homepage.index.*)': 'loadSessionsSuccess'
        'load sessions failure (scenes.homepage.index.*)': 'loadSessionsFailure'
    }
    actionTypes: {
        updateName: 'update name (scenes.homepage.index.*)'
        updateNumber: 'update number (scenes.homepage.index.*)'
        loadSessions: 'load sessions (scenes.homepage.index.*)'
        loadSessionsSuccess: 'load sessions success (scenes.homepage.index.*)'
        loadSessionsFailure: 'load sessions failure (scenes.homepage.index.*)'
    }
    actions: {
        updateName: (name: string) => void
        updateNumber: (number: number) => void
        loadSessions: (selectedDate: string) => void
        loadSessionsSuccess: (sessions: Session[]) => void
        loadSessionsFailure: (error: string) => void
    }
    cache: Record<string, any>
    connections: any
    constants: any
    defaults: {
        name: string
        number: number
        otherNameNoDefault: any
        yetAnotherNameWithNullDefault: string | null
        sessions: Session[]
        sessionsLoading: boolean
    }
    events: any
    path: ['scenes', 'homepage', 'index', '*']
    pathString: 'scenes.homepage.index.*'
    props: {
        id: number
    }
    reducer: (
        state: any,
        action: () => any,
        fullState: any,
    ) => {
        name: string
        number: number
        otherNameNoDefault: any
        yetAnotherNameWithNullDefault: string | null
        sessions: Session[]
        sessionsLoading: boolean
    }
    reducerOptions: any
    reducers: {
        name: (state: string, action: any, fullState: any) => string
        number: (state: number, action: any, fullState: any) => number
        otherNameNoDefault: (state: any, action: any, fullState: any) => any
        yetAnotherNameWithNullDefault: (state: string | null, action: any, fullState: any) => string | null
        sessions: (state: Session[], action: any, fullState: any) => Session[]
        sessionsLoading: (state: boolean, action: any, fullState: any) => boolean
    }
    selector: (
        state: any,
    ) => {
        name: string
        number: number
        otherNameNoDefault: any
        yetAnotherNameWithNullDefault: string | null
        sessions: Session[]
        sessionsLoading: boolean
    }
    selectors: {
        name: (state: any, props: any) => string
        number: (state: any, props: any) => number
        otherNameNoDefault: (state: any, props: any) => any
        yetAnotherNameWithNullDefault: (state: any, props: any) => string | null
        sessions: (state: any, props: any) => Session[]
        sessionsLoading: (state: any, props: any) => boolean
        capitalizedName: (state: any, props: any) => string
        upperCaseName: (state: any, props: any) => string
    }
    values: {
        name: string
        number: number
        otherNameNoDefault: any
        yetAnotherNameWithNullDefault: string | null
        sessions: Session[]
        sessionsLoading: boolean
        capitalizedName: string
        upperCaseName: string
    }
    _isKea: true
    __keaTypeGenInternalSelectorTypes: {
        capitalizedName: (arg1: string, arg2: number) => string
        upperCaseName: (arg1: string) => string
    }
    __keaTypeGenInternalReducerActions: {
        'set username (samples.githubLogic)': (
            username: string,
        ) => {
            type: 'set username (samples.githubLogic)'
            payload: {
                username: string
            }
        }
    }
}
