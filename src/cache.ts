import * as fs from 'fs'
import * as path from 'path'
import { AppOptions, ParsedLogic } from './types'
import { Program } from 'typescript'
import { visitProgram } from './visit/visit'

export function cachePath(appOptions: AppOptions, fileName: string): string {
    return path.join(appOptions.rootPath || process.cwd(), '.typegen', path.relative(appOptions.rootPath || process.cwd(), fileName))
}

export function restoreCachedTypes(program: Program, appOptions: AppOptions, log: (msg: string) => void): boolean {
    if (!appOptions.useCache) {
        return false
    }
    const parsedLogics = visitProgram(program, appOptions)
    let restored = false
    for (const pl of parsedLogics) {
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
