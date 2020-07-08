import { logicSourceToLogicType } from '../../utils'

test('loaders - with a function', () => {
    const logicSource = `
        import { kea } from 'kea'
        interface Session {
            user: number,
            type: string
        }
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
            }),
            loaders: ({ actions }) => ({
                sessions: {
                    __default: [] as Session[],
                    loadSessions: async (query: string): Promise<Session[]> => {
                        const response = { user: 3, type: 'bla' }
                        return [response]
                    },
                },
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toEqual(
        `
export interface logicType {
    actionCreators: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        loadSessions: (query: string) => ({
            type: string;
            payload: string;
        });
        loadSessionsSuccess: (sessions: Session[]) => ({
            type: string;
            payload: {
                sessions: Session[];
            };
        });
        loadSessionsFailure: (error: string) => ({
            type: string;
            payload: {
                error: string;
            };
        });
    };
    actions: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        loadSessions: (query: string) => ({
            type: string;
            payload: string;
        });
        loadSessionsSuccess: (sessions: Session[]) => ({
            type: string;
            payload: {
                sessions: Session[];
            };
        });
        loadSessionsFailure: (error: string) => ({
            type: string;
            payload: {
                error: string;
            };
        });
    };
    reducer: (state: any, action: () => any, fullState: any) => {
        sessions: Session[];
        sessionsLoading: boolean;
    };
    reducers: {
        sessions: (state: Session[], action: any, fullState: any) => Session[];
        sessionsLoading: (state: boolean, action: any, fullState: any) => boolean;
    };
    selector: (state: any) => {
        sessions: Session[];
        sessionsLoading: boolean;
    };
    selectors: {
        sessions: (state: any, props: any) => Session[];
        sessionsLoading: (state: any, props: any) => boolean;
    };
    values: {
        sessions: Session[];
        sessionsLoading: boolean;
    };
}`.trim(),
    )
})
