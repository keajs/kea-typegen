// Auto-generated with kea-typegen. DO NOT EDIT!

export interface githubConnectLogicType<Repository> {
    key: any;
    actionCreators: {
        setRepositories: (repositories: Repository[]) => ({
            type: "set repositories (samples.githubConnectLogic)";
            payload: {
                repositories: Repository[];
            };
        });
    };
    actionKeys: {
        "set repositories (samples.githubConnectLogic)": "setRepositories";
    };
    actionTypes: {
        setRepositories: "set repositories (samples.githubConnectLogic)";
    };
    actions: {
        setRepositories: (repositories: Repository[]) => ({
            type: "set repositories (samples.githubConnectLogic)";
            payload: {
                repositories: Repository[];
            };
        });
    };
    cache: Record<string, any>;
    connections: any;
    constants: any;
    defaults: any;
    events: any;
    path: ["samples", "githubConnectLogic"];
    pathString: "samples.githubConnectLogic";
    propTypes: any;
    props: Record<string, any>;
    reducer: (state: any, action: () => any, fullState: any) => {};
    reducerOptions: any;
    reducers: {};
    selector: (state: any) => {};
    selectors: {
        repositories: (state: any, props: any) => Repository[];
    };
    values: {
        repositories: Repository[];
    };
    _isKea: true;
}