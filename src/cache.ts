import * as fs from 'fs'
import * as path from 'path'

export interface FileCache {
    [filePath: string]: number
}

export class KeaTypegenCache {
    private cache: FileCache = {}
    private cacheFilePath: string | null = null

    constructor(rootPath: string) {
        const nodeModulesCacheDir = path.join(rootPath, 'node_modules', '.cache', 'kea-typegen')
        const nodeModulesCachePath = path.join(nodeModulesCacheDir, 'cache.json')

        try {
            fs.mkdirSync(nodeModulesCacheDir, { recursive: true })
            this.cacheFilePath = nodeModulesCachePath
        } catch (error) {
            this.cacheFilePath = null;
        }

        this.loadCache()
    }

    private loadCache(): void {
        if (!this.cacheFilePath) {
            this.cache = {};
            return;
        }

        try {
            const cacheData = fs.readFileSync(this.cacheFilePath, 'utf8')
            this.cache = JSON.parse(cacheData)
        } catch (error) {
            this.cache = {}
        }
    }

    public persistCache(): void {
        if (!this.cacheFilePath) {
            return;
        }

        try {
            fs.writeFileSync(this.cacheFilePath, JSON.stringify(this.cache, null, 2))
        } catch (error) {
            console.warn('Failed to save cache file:', error.message)
        }
    }

    public isFileChanged(filePath: string): boolean {
        try {
            const stats = fs.statSync(filePath)
            const cachedMtime = this.cache[filePath]
            return !cachedMtime || cachedMtime < stats.mtimeMs
        } catch (error) {
            return true
        }
    }


    public updateFile(filePath: string): void {
        try {
            const stats = fs.statSync(filePath)
            this.cache[filePath] = stats.mtimeMs
        } catch (error) {
            delete this.cache[filePath]
        }
    }

    public getChangedFiles(filePaths: string[]): string[] {
        return filePaths.filter(filePath => this.isFileChanged(filePath))
    }
}