interface Session {
    user: number,
    type: string
}

export interface logicType {
    actionCreators: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: string;
            payload: { otherName: string; };
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
        updateOtherName: (otherName: string) => ({
            type: string;
            payload: { otherName: string; };
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
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
        sessions: Session[];
        sessionsLoading: boolean;
    };
    reducers: {
        name: (state: string, action: any, fullState: any) => string;
        otherNameNoDefault: (state: any, action: any, fullState: any) => any;
        yetAnotherNameWithNullDefault: (state: string | null, action: any, fullState: any) => string | null;
        sessions: (state: Session[], action: any, fullState: any) => Session[];
        sessionsLoading: (state: boolean, action: any, fullState: any) => boolean;
    };
    selector: (state: any) => {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
        sessions: Session[];
        sessionsLoading: boolean;
    };
    selectors: {
        name: (state: any, props: any) => string;
        otherNameNoDefault: (state: any, props: any) => any;
        yetAnotherNameWithNullDefault: (state: any, props: any) => string | null;
        sessions: (state: any, props: any) => Session[];
        sessionsLoading: (state: any, props: any) => boolean;
        upperCaseName: (state: any, props: any) => string;
        capitalizedName: (state: any, props: any) => string;
    };
    values: {
        name: string;
        otherNameNoDefault: any;
        yetAnotherNameWithNullDefault: string | null;
        sessions: Session[];
        sessionsLoading: boolean;
        upperCaseName: string;
        capitalizedName: string;
    };
}