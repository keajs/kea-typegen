import { kea, Props, MakeLogicType } from 'kea'

interface DashboardValues {
    id: number
    created_at: string
    name: string
    pinned: boolean
}

interface DashboardActions {
    setName: (name: string) => void
}

interface DashboardProps {
    id: number
}

const makeLogicTypeLogic = kea<MakeLogicType<DashboardValues, DashboardActions, DashboardProps>>({})

makeLogicTypeLogic.actions.setName('asd')
makeLogicTypeLogic.build({  })

makeLogicTypeLogic({})
