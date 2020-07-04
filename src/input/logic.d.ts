export interface logicInterface {
    actions: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: string;
            payload: { otherName: string; };
        });
    };
    actionsCreators: {
        updateName: (name: string) => ({
            type: string;
            payload: { name: string; };
        });
        updateOtherName: (otherName: string) => ({
            type: string;
            payload: { otherName: string; };
        });
    };
}