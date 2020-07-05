// https://levelup.gitconnected.com/writing-typescript-custom-ast-transformer-part-1-7585d6916819

function compiler (configFilePath: string) {
    // tslint:disable-next-line no-any
    const host: ts.ParseConfigFileHost = ts.sys as any;
    // Fix after https://github.com/Microsoft/TypeScript/issues/18217
    host.onUnRecoverableConfigFileDiagnostic = printDiagnostic;
    const parsedCmd = ts.getParsedCommandLineOfConfigFile(configFilePath, undefined, host);
    host.onUnRecoverableConfigFileDiagnostic = undefined;

    const {options, fileNames} = parsedCmd;

    const program = ts.createProgram({
        rootNames: fileNames,
        options,
    });

    const emitResult = program.emit(
        undefined,
        undefined,
        undefined,
        undefined,
        {
            before: [],
            after: [],
            afterDeclarations: [],
        }
    );

    ts.getPreEmitDiagnostics(program)
        .concat(emitResult.diagnostics)
        .forEach(diagnostic => {
            let msg = ts.flattenDiagnosticMessageText(diagnostic.messageText, '\n');
            if (diagnostic.file) {
                const {line, character} = diagnostic.file.getLineAndCharacterOfPosition(diagnostic.start);
                msg = `${diagnostic.file.fileName} (${line + 1},${character + 1}): ${msg}`;
            }
            console.error(msg);
        });

    const exitCode = emitResult.emitSkipped ? 1 : 0;
    if (exitCode) {
        console.log(red(`Process exiting with code '${exitCode}'.`));
        process.exit(exitCode);
    }
}
