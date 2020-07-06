import { logicSourceToLogicType } from '../../index'

test('reducers', () => {
    const logicSource = `
        import { kea } from 'kea'
        
        const logic = kea({
            actions: () => ({
                updateName: (name: string) => ({ name }),
                updateOtherName: (otherName: string) => ({ otherName }),
            }),
            reducers: () => ({
                name: [
                    'birdname',
                    {
                        updateName: (_, { name }) => name,
                        [actions.updateOtherName]: (state, payload) => payload.name,
                    },
                ],
                otherNameNoDefault: {
                    updateName: (_, { name }) => name,
                },
                yetAnotherNameWithNullDefault: [
                    null as (string | null),
                    {
                        updateName: (_, { name }) => name,
                    },
                ],
            })
        })
    `
    expect(logicSourceToLogicType(logicSource)).toEqual(
        `
export interface logicType {
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
    reducers: {
        name: () => string;
        otherNameNoDefault: () => any;
        yetAnotherNameWithNullDefault: () => (string | null);
    };
}`.trim(),
    )
})
