// Auto-generated with kea-typegen. DO NOT EDIT!

export interface logicType<Repository> {
    key: any;
    actionCreators: {
        setUsername: (username: string) => ({
            type: "set username (samples.github)";
            payload: { username: string; };
        });
        setRepositories: (repositories: Repository[]) => ({
            type: "set repositories (samples.github)";
            payload: { repositories: Repository[]; };
        });
        setFetchError: (error: string) => ({
            type: "set fetch error (samples.github)";
            payload: { error: string; };
        });
    };
    actionKeys: Record<string, any>;
    actionTypes: Record<string, any>;
    actions: {
        setUsername: (username: string) => ({
            type: "set username (samples.github)";
            payload: { username: string; };
        });
        setRepositories: (repositories: Repository[]) => ({
            type: "set repositories (samples.github)";
            payload: { repositories: Repository[]; };
        });
        setFetchError: (error: string) => ({
            type: "set fetch error (samples.github)";
            payload: { error: string; };
        });
    };
    cache: Record<string, any>;
    connections: any;
    constants: any;
    defaults: any;
    events: any;
    path: ["samples", "github"];
    pathString: "samples.github";
    propTypes: any;
    props: Record<string, any>;
    reducer: (state: any, action: () => any, fullState: any) => {
        username: string;
        repositories: Repository[];
        isLoading: boolean;
        error: string | null;
    };
    reducerOptions: any;
    reducers: {
        username: (state: string, action: any, fullState: any) => string;
        repositories: (state: Repository[], action: any, fullState: any) => Repository[];
        isLoading: (state: boolean, action: any, fullState: any) => boolean;
        error: (state: string | null, action: any, fullState: any) => string | null;
    };
    selector: (state: any) => {
        username: string;
        repositories: Repository[];
        isLoading: boolean;
        error: string | null;
    };
    selectors: {
        username: (state: any, props: any) => string;
        repositories: (state: any, props: any) => Repository[];
        isLoading: (state: any, props: any) => boolean;
        error: (state: any, props: any) => string | null;
        sortedRepositories: (state: any, props: any) => Repository[];
    };
    values: {
        username: string;
        repositories: Repository[];
        isLoading: boolean;
        error: string | null;
        sortedRepositories: Repository[];
    };
    _isKea: true;
    __selectorTypeHelp: {
        sortedRepositories: (arg1: Repository[]) => Repository[];
    };
}