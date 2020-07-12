export interface logicType<Session> {
    actionCreators: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        updateNumber: (number: number) => ({
            type: string;
            payload: { number: number; };
        });
        loadSessions: (selectedDate: any) => ({
            type: string;
            payload: any;
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
        updateNumber: (number: number) => ({
            type: string;
            payload: { number: number; };
        });
        loadSessions: (selectedDate: any) => ({
            type: string;
            payload: any;
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
        name: string;
        number: number;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
        sessions: Session[];
        sessionsLoading: boolean;
    };
    reducers: {
        name: (state: string, action: any, fullState: any) => string;
        number: (state: number, action: any, fullState: any) => number;
        otherNameNoDefault: (state: any, action: any, fullState: any) => any;
        yetAnotherNameWithNullDefault: (state: string | null, action: any, fullState: any) => string | null;
        sessions: (state: Session[], action: any, fullState: any) => Session[];
        sessionsLoading: (state: boolean, action: any, fullState: any) => boolean;
    };
    selector: (state: any) => {
        name: string;
        number: number;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
        sessions: Session[];
        sessionsLoading: boolean;
    };
    selectors: {
        name: (state: any, props: any) => string;
        number: (state: any, props: any) => number;
        otherNameNoDefault: (state: any, props: any) => any;
        yetAnotherNameWithNullDefault: (state: any, props: any) => string | null;
        sessions: (state: any, props: any) => Session[];
        sessionsLoading: (state: any, props: any) => boolean;
        upperCaseName: (state: any, props: any) => any;
        capitalizedName: (state: any, props: any) => any;
    };
    values: {
        name: string;
        number: number;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
        sessions: Session[];
        sessionsLoading: boolean;
        upperCaseName: any;
        capitalizedName: any;
    };
}