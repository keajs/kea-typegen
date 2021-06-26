// actions
export interface A1 {
    bla: string
}
export type A2 = 'A2'
export type A3 = string | boolean
export type A4 = Record<string, boolean>
export interface A5 {
    bla: string
    foo: boolean
    a6: A6
}
export type A6 = Partial<Record<string, A7>>
export type A7 = Array<string>

// defaults
export interface D1 {
    bla: string
}
export type D2 = 'D2'
export type D3 = string | boolean
export type D4 = Record<string, boolean>
export interface D5 {
    bla: string
    foo: boolean
    a6: D6
}
export type D6 = Partial<Record<string, D7>>
export type D7 = Array<string>

// reducers
export interface R1 {
    bla: string
}
export type R2 = 'R2'
export type R3 = string | boolean
export type R4 = Record<string, boolean>
export interface R5 {
    bla: string
    foo: boolean
    a6: R6
}
export type R6 = Partial<Record<string, R7>>
export type R7 = Array<string>

// selectors
export interface S1 {
    bla: string
}
export type S2 = 'S2'
export type S3 = string | boolean
export type S4 = Record<string, boolean>
export interface S5 {
    bla: string
    foo: boolean
    a6: S6
}
export type S6 = Partial<Record<string, S7>>
export type S7 = Array<string>

export class EventIndex {}

export declare namespace ExportedApi {
    interface RandomThing {
        key: string
    }
}
