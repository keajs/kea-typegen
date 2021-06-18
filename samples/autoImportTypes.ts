// export type T1 = 't1'
export interface T1 {
    bla: string
}
export type T2 = 't2'
export type T3 = string | boolean
export type T4 = Record<string, boolean>
export interface T5 {
    bla: string
    foo: boolean
    t6: T6
}
export type T6 = Partial<Record<string, T7>>
export type T7 = Array<string>
