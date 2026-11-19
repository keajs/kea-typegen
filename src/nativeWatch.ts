import * as ts from 'typescript'
import { AppOptions } from './types'

type WatchProgramFiles = (files: readonly string[]) => void

/** A native Program owns no file watchers. Keep the watch set across passes and
 * release the compiler (including its Go process) as soon as the writers finish. */
export async function watchNativeProject(
    options: AppOptions,
    generate: (watchProgramFiles: WatchProgramFiles) => Promise<void>,
) {
    const watchers = new Map<string, ts.FileWatcher>()
    let timer: NodeJS.Timeout
    let pending = false
    let running = false
    let closed = false

    function schedule() {
        if (closed) return
        pending = true
        clearTimeout(timer)
        timer = setTimeout(run, 200)
    }

    function refreshWatches(files: readonly string[]) {
        if (closed) return
        const wanted = new Set<string>()
        function watch(key: string, create: () => ts.FileWatcher) {
            wanted.add(key)
            if (!watchers.has(key)) watchers.set(key, create())
        }
        function watchFile(file: string) {
            watch(file, () => ts.sys.watchFile(file, schedule))
        }
        files.forEach(watchFile)
        if (options.sourceFilePath) watchFile(options.sourceFilePath)
        if (options.tsConfigPath) {
            // The classic config reader exposes the wildcard directories needed to discover
            // new root files, including patterns inherited from configs in other directories.
            // The native compiler already enumerated the files and owns compiler options.
            const config = ts.getParsedCommandLineOfConfigFile(
                options.tsConfigPath,
                {},
                {
                    ...ts.sys,
                    onUnRecoverableConfigFileDiagnostic() {},
                    readDirectory: () => [],
                    readFile(name) {
                        watchFile(name)
                        return ts.sys.readFile(name)
                    },
                },
            )
            for (const [directory, flags] of Object.entries(config?.wildcardDirectories ?? {})) {
                const recursive = flags === ts.WatchDirectoryFlags.Recursive
                watch(`dir:${recursive}:${directory}`, () =>
                    ts.sys.watchDirectory(
                        directory,
                        (name) => {
                            if (/\.(?:[cm]?[jt]sx?|json)$/i.test(name) || ts.sys.directoryExists(name)) schedule()
                        },
                        recursive,
                        config.watchOptions,
                    ),
                )
            }
        }
        for (const [key, handle] of watchers) {
            if (!wanted.has(key)) {
                handle.close()
                watchers.delete(key)
            }
        }
    }

    async function run() {
        if (closed || running) return
        running = true
        pending = false
        try {
            await generate(refreshWatches)
        } catch (error) {
            options.log(`Native typegen watch pass failed: ${error}`)
        } finally {
            running = false
            if (pending) schedule()
        }
    }

    refreshWatches([])
    await run()
    return {
        close() {
            closed = true
            clearTimeout(timer)
            for (const handle of watchers.values()) handle.close()
            watchers.clear()
        },
    }
}
