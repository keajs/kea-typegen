export type Repository = {
    id: number
    stargazers_count: number
    html_url: string
    full_name: string
    forks: number
}

export interface Dashboard {
    id: number
    created_at?: string
    name?: string
    pinned?: boolean
}
