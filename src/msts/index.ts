// https://github.com/Microsoft/TypeScript/wiki/Using-the-Compiler-API#using-the-type-checker
import * as ts from "typescript";
import * as fs from "fs";

interface DocEntry {
    name?: string;
    fileName?: string;
    documentation?: string;
    type?: string;
    constructors?: DocEntry[];
    parameters?: DocEntry[];
    returnType?: string;
}

/** Generate documentation for all classes in a set of .ts files */
function checkLogics(
    fileNames: string[],
    options: ts.CompilerOptions
): void {
    // Build a program using the set of root file names in fileNames
    let program = ts.createProgram(fileNames, options);

    // Get the checker, we will use it to find more about classes
    let checker = program.getTypeChecker();
    let output: DocEntry[] = [];

    // Visit every sourceFile in the program
    for (const sourceFile of program.getSourceFiles()) {
        if (!sourceFile.isDeclarationFile) {
            // console.log(sourceFile)
            // Walk the tree to search for classes
            ts.forEachChild(sourceFile, visit);
        }
    }

    // print out the doc
    fs.writeFileSync("classes.json", JSON.stringify(output, undefined, 4));

    return;

    /** visit nodes finding exported classes */
    function visit(node: ts.Node) {
        if (!ts.isIdentifier(node)) {
            ts.forEachChild(node, visit);
            return
        }

        if (!node.parent || !ts.isCallExpression(node.parent)) {
            return
        }

        const symbol = checker.getSymbolAtLocation(node);
        if (!symbol || symbol.getName() !== 'kea') {
            return
        }

        const input = node.parent.arguments[0]

        if (!ts.isObjectLiteralExpression(input)) {
            return
        }

        // ts.get
        const symbol3 = checker.getSymbolAtLocation(input);

        const properties = input.properties as unknown as ts.PropertySignature[]

        for (const property of properties) {
            const symbol = checker.getSymbolAtLocation(property.name as ts.Identifier);
            const name = symbol?.getName()

            if (symbol && name === 'actions') {
                const type = checker.getTypeOfSymbolAtLocation(symbol, symbol.valueDeclaration!)
                const typeNode = checker.typeToTypeNode(type) as ts.TypeNode
                console.log(checker.typeToString(type))

                if (ts.isFunctionTypeNode(typeNode)) {
                    const signature = type.getCallSignatures()[0]
                    const returnType = signature.getReturnType()
                    console.log(checker.typeToString(returnType))
                    debugger

                }

                debugger

            }

            // const value = property.initializer as ts.Node
            //
            // if (ts.isArrowFunction(value)) {
            //     symbol.valueDeclaration
            //     // returnType: checker.typeToString(signature.getReturnType()),
            //
            //
            //     debugger
            // }
            //
            // if (name === 'actions') {
            //     const value = property.initializer
            //     debugger
            // }
        }

        console.log('!!!')


        debugger

    }

    /** Serialize a symbol into a json object */
    function serializeSymbol(symbol: ts.Symbol): DocEntry {
        return {
            name: symbol.getName(),
            type: checker.typeToString(
                checker.getTypeOfSymbolAtLocation(symbol, symbol.valueDeclaration!)
            )
        };
    }

    // /** Serialize a class symbol information */
    // function serializeClass(symbol: ts.Symbol) {
    //     let details = serializeSymbol(symbol);
    //
    //     // Get the construct signatures
    //     let constructorType = checker.getTypeOfSymbolAtLocation(
    //         symbol,
    //         symbol.valueDeclaration!
    //     );
    //     details.constructors = constructorType
    //         .getConstructSignatures()
    //         .map(serializeSignature);
    //     return details;
    // }
    //
    // /** Serialize a signature (call or construct) */
    // function serializeSignature(signature: ts.Signature) {
    //     return {
    //         parameters: signature.parameters.map(serializeSymbol),
    //         returnType: checker.typeToString(signature.getReturnType()),
    //         documentation: ts.displayPartsToString(signature.getDocumentationComment(checker))
    //     };
    // }

    /** True if this is visible outside this file, false otherwise */
    function isNodeExported(node: ts.Node): boolean {
        return (
            (ts.getCombinedModifierFlags(node as ts.Declaration) & ts.ModifierFlags.Export) !== 0 ||
            (!!node.parent && node.parent.kind === ts.SyntaxKind.SourceFile)
        );
    }
}

checkLogics(["./src/input/logic.ts"], {
    target: ts.ScriptTarget.ES5,
    module: ts.ModuleKind.CommonJS
});