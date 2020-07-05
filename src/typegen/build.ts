import * as ts from "typescript";
import fs from "fs";
import {ParsedLogic} from "./types";

// function buildLogicType(fileName: string, parsedLogics: ParsedLogic[]) {
//     const printer = ts.createPrinter({newLine: ts.NewLineKind.LineFeed})
//
//
//     for (const parsedLogic of parsedLogics) {
//         const logicType = createLogicType(parsedLogic)
//         const result = printer.printNode(ts.EmitHint.Unspecified, logicType, resultFile)
//     }
//     console.log(result)
//     fs.writeFileSync(resultFile.fileName, result)
//
//     printer.printFile(resultFile)
//     // checker.typeToString(logicType)
//     // debugger
// }

export function logicToTypeString(parsedLogic: ParsedLogic) {
    const logicType = createLogicType(parsedLogic)
    const printer = ts.createPrinter({newLine: ts.NewLineKind.LineFeed})
    const sourceFile = ts.createSourceFile(
        'logic.ts',
        '',
        ts.ScriptTarget.Latest,
        /*setParentNodes*/ false,
        ts.ScriptKind.TS,
    )
    return printer.printNode(ts.EmitHint.Unspecified, logicType, sourceFile)
}

export function createLogicType(parsedLogic: ParsedLogic) {
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
                ts.createTypeLiteralNode(createActions(parsedLogic)),
                undefined,
            ),
            ts.createPropertySignature(
                undefined,
                ts.createIdentifier('actionsCreators'),
                undefined,
                ts.createTypeLiteralNode(createActions(parsedLogic)),
                undefined,
            ),
        ],
    )
}

function createActions(parsedLogic: ParsedLogic) {
    return parsedLogic.actions.map((action) => {
        // ts.getMutableClone(action.signature./getReturnType().)

        const returnType = action.signature.getReturnType()
        const parameters = action.signature.getDeclaration().parameters

        // ...parameters.map(checker.typeToTypeNode),
        const params = (parameters.map((param) => {
            // const type = checker.getTypeOfSymbolAtLocation(param, param.valueDeclaration!)
            // const typeNode = checker.typeToTypeNode(type) as ts.TypeNode
            // // ts.getMutableClone()
            // return typeNode
            return param
        }) as unknown) as ts.ParameterDeclaration[]

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
                            parsedLogic.checker.typeToTypeNode(returnType),
                            undefined,
                        ),
                    ]),
                ),
            ),
            undefined,
        )
    })
}
