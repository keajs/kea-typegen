import * as fs from 'fs'
import * as path from 'path'
import { AppOptions, ParsedLogic } from './types'
import { Program } from 'typescript'
import { visitProgram } from './visit/visit'
import { isInlineFile } from './utils'

export function cachePath(appOptions: AppOptions, fileName: string): string {
    return path.join(process.cwd(), '.typegen', path.relative(process.cwd(), fileName))
}

export function restoreCachedTypes(program: Program, appOptions: AppOptions, log: (msg: string) => void): boolean {
    if (!appOptions.useCache) {
        return false
    }
    const parsedLogics = visitProgram(program, appOptions)
    const parsedLogicsByFile: Record<string, ParsedLogic[]> = {}
    for (const pl of parsedLogics) {
        parsedLogicsByFile[pl.fileName] = [...(parsedLogicsByFile[pl.fileName] ?? []), pl]
    }
    let restored = false
    for (const pl of parsedLogics) {
        if (isInlineFile(appOptions, program.getSourceFile(pl.fileName), parsedLogicsByFile[pl.fileName])) {
            // inline files have their types in the logic file itself, there is no logicType.ts to restore
            continue
        }
        if (!fs.existsSync(pl.typeFileName)) {
            const from = cachePath(appOptions, pl.typeFileName)
            if (fs.existsSync(from)) {
                fs.mkdirSync(path.dirname(pl.typeFileName), { recursive: true })
                fs.copyFileSync(from, pl.typeFileName)
                log(`♻️ Restored from cache: ${path.relative(process.cwd(), pl.typeFileName)}`)
                restored = true
            }
        }
    }
    return restored
}

export function cacheWrittenFile(fileName: string, appOptions: AppOptions) {
    if (!appOptions.useCache) {
        return
    }
    const dest = cachePath(appOptions, fileName)
    fs.mkdirSync(path.dirname(dest), { recursive: true })
    fs.copyFileSync(fileName, dest)
}

export function deleteCachedFile(fileName: string, appOptions: AppOptions) {
    if (!appOptions.useCache) {
        return
    }
    const dest = cachePath(appOptions, fileName)
    if (fs.existsSync(dest)) {
        fs.unlinkSync(dest)
    }
}
