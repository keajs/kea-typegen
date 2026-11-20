import type * as ts from 'typescript'

export const sourceExtensions = ['.ts', '.tsx', '.mts', '.cts', '.js', '.jsx', '.mjs', '.cjs', '.mjsx']

// The visitors also accept a classic ts.Program (used by watch mode and by API consumers).
export type OutputProgram = Pick<ts.Program, 'getCompilerOptions' | 'getSourceFile'> & {
    requiresImportExtension?(fileName: string, fallbackFile?: string): boolean
} & (
        | { getSourceFileNames(): readonly string[]; readSourceText(fileName: string): string | undefined }
        | Pick<ts.Program, 'getSourceFiles'>
    )

export type LogicProgram = OutputProgram & { getTypeChecker(sourceFile?: ts.SourceFile): ts.TypeChecker }

export function sourceFileNames(program: OutputProgram): readonly string[] {
    return 'getSourceFileNames' in program
        ? program.getSourceFileNames()
        : program.getSourceFiles().map((file) => file.fileName)
}
