import { kea } from 'kea'
import { propsLogicType } from './propsLogicType'

const propsLogic = kea<propsLogicType>({
    props: {
        /* TODO for 3.0: set default props here */
    } as {
        page: string
        id: number
    },

    key: (props) => props.id,

    actions: {
        setPage: (page: string) => ({ page }),
        setId: (id: number) => ({ id }),
    },

    reducers: ({ props }) => ({
        currentPage: [
            props.page,
            {
                setPage: (_, { page }) => page,
            },
        ],
    }),
})
