// https://github.com/Microsoft/TypeScript/wiki/Using-the-Compiler-API#using-the-type-checker
import * as ts from 'typescript'
import * as fs from 'fs'

interface ActionTransform {
    name: string
    type: ts.Type
    signature: ts.Signature
    typeNode: ts.TypeNode
}

interface ReducerTransform {
    name: string
    type: ts.Type | undefined
}

/** Generate documentation for all classes in a set of .ts files */
function checkLogics(fileNames: string[], options: ts.CompilerOptions): void {
    // Build a program using the set of root file names in fileNames
    let program = ts.createProgram(fileNames, options)

    // Get the checker, we will use it to find more about classes
    let checker = program.getTypeChecker()
    let actions: ActionTransform[]
    let reducers: ReducerTransform[]
    // Visit every sourceFile in the program
    for (const sourceFile of program.getSourceFiles()) {
        if (!sourceFile.isDeclarationFile) {
            // console.log(sourceFile)
            // Walk the tree to search for classes
            // let output: DocEntry[] = []
            actions = []
            reducers = []

            ts.forEachChild(sourceFile, visit)

            if (actions.length > 0 || reducers.length > 0) {
                const logicType = createLogicType()

                const resultFile = ts.createSourceFile(
                    sourceFile.fileName.replace('.ts', '.d.ts'),
                    '',
                    ts.ScriptTarget.Latest,
                    /*setParentNodes*/ false,
                    ts.ScriptKind.TS,
                )
                const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed })
                const result = printer.printNode(ts.EmitHint.Unspecified, logicType, resultFile)
                console.log(result)
                fs.writeFileSync(resultFile.fileName, result);

                printer.printFile(resultFile)
                // checker.typeToString(logicType)
                // debugger
            }
        }
    }

    // print out the doc
    // fs.writeFileSync('classes.json', JSON.stringify(output, undefined, 4))

    return

    /** visit nodes finding exported classes */
    function visit(node: ts.Node) {
        if (!ts.isIdentifier(node)) {
            ts.forEachChild(node, visit)
            return
        }

        if (!node.parent || !ts.isCallExpression(node.parent)) {
            return
        }

        const symbol = checker.getSymbolAtLocation(node)
        if (!symbol || symbol.getName() !== 'kea') {
            return
        }

        const input = node.parent.arguments[0]

        if (!ts.isObjectLiteralExpression(input)) {
            return
        }

        // ts.get
        const symbol3 = checker.getSymbolAtLocation(input)

        const properties = (input.properties as unknown) as ts.PropertySignature[]

        for (const property of properties) {
            const symbol = checker.getSymbolAtLocation(property.name as ts.Identifier)

            if (!symbol) {
                continue
            }

            const name = symbol?.getName()
            let type = checker.getTypeOfSymbolAtLocation(symbol, symbol.valueDeclaration!)

            if (ts.isFunctionTypeNode(checker.typeToTypeNode(type) as ts.TypeNode)) {
                type = type.getCallSignatures()[0].getReturnType()
            }
            console.log({ name, type: checker.typeToString(type) })

            if (name === 'actions') {
                const properties = checker.getPropertiesOfType(type)

                for (const property of properties) {
                    const name = property.getName()
                    const type = checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration!)
                    const typeNode = checker.typeToTypeNode(type) as ts.TypeNode
                    const signature = type.getCallSignatures()[0]

                    actions.push({name, type, typeNode, signature})

                    // console.log(checker.typeToString(type))
                    // const returnType = signature.getReturnType()
                    // console.log(checker.typeToString(returnType))
                }

            } else if (name === 'reducers') {
                for (const property of type.getProperties()) {
                    parseReducerProperty(property)
                }
            }
        }
    }

    function parseReducerProperty (property: ts.Symbol) {
        const name = property.getName()
        const type = checker.getTypeOfSymbolAtLocation(property, property.valueDeclaration!)
        console.log(checker.typeToString(type))
        const typeNode = checker.typeToTypeNode(type) as ts.TypeNode

        // typeNode.elementType.type.types[0]

        // if (ts.IntersectionTypeNode(typeNode)){
        //     //typeNode.
        //     // const typeOfFirstInArray = (type as ts.TypeReference).resolvedTypeArguments?.[0]?.types?.[0]
        //
        //     // reducers.push({name, type: typeOfFirstInArray})
        //     console.log('.')
        // }

        console.log('.')
    }

    function createLogicType() {
        return ts.createInterfaceDeclaration(
            undefined,
            [ts.createModifier(ts.SyntaxKind.ExportKeyword)],
            ts.createIdentifier('logicInterface'),
            undefined,
            undefined,
            [
                ts.createPropertySignature(
                    undefined,
                    ts.createIdentifier('actions'),
                    undefined,
                    ts.createTypeLiteralNode(createActions(actions)),
                    undefined,
                ),
                ts.createPropertySignature(
                    undefined,
                    ts.createIdentifier('actionsCreators'),
                    undefined,
                    ts.createTypeLiteralNode(createActions(actions)),
                    undefined,
                ),
            ],
        )
    }

    function createActions(actions: ActionTransform[]) {
        return actions.map((action) => {
            // ts.getMutableClone(action.signature./getReturnType().)

            const returnType = action.signature.getReturnType()
            const parameters = action.signature.getDeclaration().parameters

            // ...parameters.map(checker.typeToTypeNode),
            const params = parameters.map(param => {
                // const type = checker.getTypeOfSymbolAtLocation(param, param.valueDeclaration!)
                // const typeNode = checker.typeToTypeNode(type) as ts.TypeNode
                // // ts.getMutableClone()
                // return typeNode
                return param
            }) as unknown as ts.ParameterDeclaration[]

            // console.log(checker.typeToString(action.type))
            // console.log(checker.typeToString(returnType))
            // debugger
            return ts.createPropertySignature(
                undefined,
                ts.createIdentifier(action.name),
                undefined,
                ts.createFunctionTypeNode(
                    undefined,
                    params,
                    ts.createParenthesizedType(
                        ts.createTypeLiteralNode([
                            ts.createPropertySignature(
                                undefined,
                                ts.createIdentifier('type'),
                                undefined,
                                ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                                undefined,
                            ),
                            ts.createPropertySignature(
                                undefined,
                                ts.createIdentifier('payload'),
                                undefined,
                                checker.typeToTypeNode(returnType),
                                undefined,
                            ),
                        ]),
                    ),
                ),
                undefined,
            )
        })
    }

    /** Serialize a symbol into a json object */
    function serializeSymbol(symbol: ts.Symbol): any {
        return {
            name: symbol.getName(),
            type: checker.typeToString(checker.getTypeOfSymbolAtLocation(symbol, symbol.valueDeclaration!)),
        }
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
        )
    }
}

checkLogics(['./src/input/logic.ts'], {
    target: ts.ScriptTarget.ES5,
    module: ts.ModuleKind.CommonJS,
})
