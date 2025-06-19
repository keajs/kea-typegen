import * as fs from 'fs'
import * as path from 'path'
import * as crypto from 'crypto'
import { version } from '../package.json'

export interface FileCache {
    [filePath: string]: string
}

export interface CacheData {
    version: string
    files: FileCache
}

export class KeaTypegenCache {
    private cache: FileCache = {}
    private cacheFilePath: string | null = null
    private rootPath: string
    private cacheVersion: string = version

    constructor(rootPath: string) {
        this.rootPath = rootPath;
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
            const rawData = fs.readFileSync(this.cacheFilePath, 'utf8')
            const cacheData: CacheData = JSON.parse(rawData)
            
            // Invalidate cache if version changed
            if (cacheData.version === this.cacheVersion) {
                this.cache = cacheData.files || {}
            } else {
                this.cache = {}
            }
        } catch (error) {
            this.cache = {}
        }
    }

    public persistCache(): void {
        if (!this.cacheFilePath) {
            return;
        }

        try {
            const cacheData: CacheData = {
                version: this.cacheVersion,
                files: this.cache
            }
            fs.writeFileSync(this.cacheFilePath, JSON.stringify(cacheData, null, 2))
        } catch (error) {
            console.warn('Failed to save cache file:', error.message)
        }
    }

    private getFileHash(filePath: string): string {
        const content = fs.readFileSync(filePath)
        return crypto.createHash('md5').update(content).digest('hex')
    }

    private toRelativePath(absolutePath: string): string {
        return path.relative(this.rootPath, absolutePath)
    }

    public isFileChanged(filePath: string): boolean {
        try {
            const relativePath = this.toRelativePath(filePath)
            const currentHash = this.getFileHash(filePath)
            const cachedHash = this.cache[relativePath]
            return !cachedHash || cachedHash !== currentHash
        } catch (error) {
            return true
        }
    }


    public updateFile(filePath: string): void {
        try {
            const relativePath = this.toRelativePath(filePath)
            this.cache[relativePath] = this.getFileHash(filePath)
        } catch (error) {
            const relativePath = this.toRelativePath(filePath)
            delete this.cache[relativePath]
        }
    }

    public getChangedFiles(filePaths: string[]): string[] {
        return filePaths.filter(filePath => this.isFileChanged(filePath))
    }
}