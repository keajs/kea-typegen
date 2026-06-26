import * as fs from 'fs'
import * as path from 'path'
import * as ts from 'typescript'
import { getFilenameForImportDeclaration } from './utils'

const KEA_CALL_REGEX = /\bkea\s*[<(]/
const SOURCE_EXTENSIONS = ['.ts', '.tsx', '.js', '.jsx', '.mjs', '.mjsx']
const TYPEGEN_FILE_REGEX = /(^|\.|\/)typegen\.[tj]s$/
const MAX_INCREMENTAL_CHANGED_FILES = 50

export interface WatchFileChange {
    fileName: string
    eventKind: ts.FileWatcherEventKind
}

export type WatchTypegenPlan =
    | { kind: 'full'; reason: string }
    | { kind: 'files'; sourceFilePaths: string[]; reason: string }
    | { kind: 'skip'; reason: string }

export interface WatchChangeTracker {
    system: ts.System
    consume(): WatchFileChange[]
}

export function createWatchChangeTracker(baseSystem: ts.System): WatchChangeTracker {
    const changes = new Map<string, WatchFileChange>()

    const track = (fileName: string, eventKind: ts.FileWatcherEventKind): void => {
        changes.set(path.resolve(fileName), { fileName: path.resolve(fileName), eventKind })
    }

    const system: ts.System = {
        ...baseSystem,
        watchFile(fileName, callback, pollingInterval, options) {
            return baseSystem.watchFile!(
                fileName,
                (changedFileName, eventKind, modifiedTime) => {
                    track(changedFileName, eventKind)
                    callback(changedFileName, eventKind, modifiedTime)
                },
                pollingInterval,
                options,
            )
        },
        watchDirectory(fileName, callback, recursive, options) {
            return baseSystem.watchDirectory!(
                fileName,
                (changedFileName) => {
                    track(changedFileName, ts.FileWatcherEventKind.Created)
                    callback(changedFileName)
                },
                recursive,
                options,
            )
        },
    }

    return {
        system,
        consume() {
            const currentChanges = [...changes.values()]
            changes.clear()
            return currentChanges
        },
    }
}

export function planWatchTypegenPass(program: ts.Program, changes: WatchFileChange[]): WatchTypegenPlan {
    if (changes.length === 0) {
        return { kind: 'full', reason: 'initial watch pass' }
    }

    const normalizedChanges = dedupeChanges(changes)

    if (normalizedChanges.length > MAX_INCREMENTAL_CHANGED_FILES) {
        return { kind: 'full', reason: `${normalizedChanges.length} files changed` }
    }

    if (normalizedChanges.some((change) => change.eventKind === ts.FileWatcherEventKind.Deleted)) {
        return { kind: 'full', reason: 'deleted file detected' }
    }

    const sourceFilesByPath = getSourceFilesByPath(program)
    const reverseDependencies = buildReverseDependencyMap(program)
    const queue: string[] = []

    for (const change of normalizedChanges) {
        const changedPath = path.resolve(change.fileName)

        if (isTypegenModuleFile(changedPath)) {
            return { kind: 'full', reason: `typegen module changed: ${path.relative(process.cwd(), changedPath)}` }
        }

        const sourceFile = sourceFilesByPath.get(changedPath)

        if (sourceFile?.text.includes('resetContext')) {
            return { kind: 'full', reason: `resetContext changed: ${path.relative(process.cwd(), changedPath)}` }
        }

        if (isGeneratedTypeFile(changedPath)) {
            queue.push(...sourcePathsForTypeFile(changedPath, sourceFilesByPath))
            queue.push(changedPath)
            continue
        }

        if (sourceFile) {
            if (sourceFile.isDeclarationFile) {
                return {
                    kind: 'full',
                    reason: `declaration file changed: ${path.relative(process.cwd(), changedPath)}`,
                }
            }

            queue.push(changedPath)
            continue
        }

        if (isSourceLikeFile(changedPath) && fs.existsSync(changedPath)) {
            return {
                kind: 'full',
                reason: `new source file outside program: ${path.relative(process.cwd(), changedPath)}`,
            }
        }
    }

    const affectedLogicFiles = collectAffectedLogicFiles(queue, sourceFilesByPath, reverseDependencies)

    if (affectedLogicFiles.length === 0) {
        return { kind: 'skip', reason: 'no affected Kea logic files' }
    }

    return {
        kind: 'files',
        sourceFilePaths: affectedLogicFiles,
        reason: `${affectedLogicFiles.length} affected Kea logic file${affectedLogicFiles.length === 1 ? '' : 's'}`,
    }
}

function dedupeChanges(changes: WatchFileChange[]): WatchFileChange[] {
    const deduped = new Map<string, WatchFileChange>()

    for (const change of changes) {
        deduped.set(path.resolve(change.fileName), { ...change, fileName: path.resolve(change.fileName) })
    }

    return [...deduped.values()]
}

function getSourceFilesByPath(program: ts.Program): Map<string, ts.SourceFile> {
    const sourceFilesByPath = new Map<string, ts.SourceFile>()

    for (const sourceFile of program.getSourceFiles()) {
        sourceFilesByPath.set(path.resolve(sourceFile.fileName), sourceFile)
    }

    return sourceFilesByPath
}

function buildReverseDependencyMap(program: ts.Program): Map<string, Set<string>> {
    const checker = program.getTypeChecker()
    const reverseDependencies = new Map<string, Set<string>>()

    for (const sourceFile of program.getSourceFiles()) {
        if (sourceFile.isDeclarationFile) {
            continue
        }

        const importerPath = path.resolve(sourceFile.fileName)

        ts.forEachChild(sourceFile, function visit(node) {
            if (ts.isImportDeclaration(node)) {
                const importedFileName = getFilenameForImportDeclaration(checker, node)

                if (importedFileName) {
                    const importedPath = path.resolve(importedFileName)
                    let importers = reverseDependencies.get(importedPath)

                    if (!importers) {
                        importers = new Set()
                        reverseDependencies.set(importedPath, importers)
                    }

                    importers.add(importerPath)
                }
            }

            ts.forEachChild(node, visit)
        })
    }

    return reverseDependencies
}

function collectAffectedLogicFiles(
    startPaths: string[],
    sourceFilesByPath: Map<string, ts.SourceFile>,
    reverseDependencies: Map<string, Set<string>>,
): string[] {
    const visited = new Set<string>()
    const queue = startPaths.map((filePath) => path.resolve(filePath))
    const affectedLogicFiles = new Set<string>()

    while (queue.length > 0) {
        const currentPath = queue.shift()!

        if (visited.has(currentPath)) {
            continue
        }

        visited.add(currentPath)

        const sourceFile = sourceFilesByPath.get(currentPath)
        if (
            sourceFile &&
            !sourceFile.isDeclarationFile &&
            !isGeneratedTypeFile(currentPath) &&
            sourceFileMightContainKeaCall(sourceFile)
        ) {
            affectedLogicFiles.add(currentPath)
        }

        if (isGeneratedTypeFile(currentPath)) {
            queue.push(...sourcePathsForTypeFile(currentPath, sourceFilesByPath))
        }

        for (const importerPath of reverseDependencies.get(currentPath) ?? []) {
            queue.push(importerPath)
        }
    }

    return [...affectedLogicFiles].sort()
}

function sourcePathsForTypeFile(typeFilePath: string, sourceFilesByPath: Map<string, ts.SourceFile>): string[] {
    if (!isGeneratedTypeFile(typeFilePath)) {
        return []
    }

    const sourceBasePath = typeFilePath.slice(0, -'Type.ts'.length)

    return SOURCE_EXTENSIONS.map((extension) => `${sourceBasePath}${extension}`).filter((sourcePath) =>
        sourceFilesByPath.has(path.resolve(sourcePath)),
    )
}

function isGeneratedTypeFile(filePath: string): boolean {
    return filePath.endsWith('Type.ts')
}

function isSourceLikeFile(filePath: string): boolean {
    return SOURCE_EXTENSIONS.some((extension) => filePath.endsWith(extension))
}

function isTypegenModuleFile(filePath: string): boolean {
    return TYPEGEN_FILE_REGEX.test(filePath.split(path.sep).join('/'))
}

function sourceFileMightContainKeaCall(sourceFile: ts.SourceFile): boolean {
    return KEA_CALL_REGEX.test(sourceFile.text)
}
