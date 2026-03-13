export type APIScopeObject = 'project' | 'feature_flag'

export type ProjectType = {
    id: number
    name: string
}

export type GroupTypeIndex = number

export type GroupType = {
    group_type_index: GroupTypeIndex
    name: string
}

export type FeatureFlagsResult = {
    results: { key: string }[]
}

export type ExperimentsResult = {
    results: { feature_flag_key: string }[]
}
