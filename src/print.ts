import * as ts from 'typescript'
import { ParsedLogic } from './types'

export function logicToTypeString(parsedLogic: ParsedLogic) {
    const logicType = createLogicType(parsedLogic)
    const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed })
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
        ts.createIdentifier(`${parsedLogic.logicName}Type`),
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
            ts.createPropertySignature(
                undefined,
                ts.createIdentifier('reducers'),
                undefined,
                ts.createTypeLiteralNode(createReducers(parsedLogic)),
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

function createReducers(parsedLogic: ParsedLogic) {
    return parsedLogic.reducers.map((reducer) => {
        return ts.createPropertySignature(
            undefined,
            ts.createIdentifier(reducer.name),
            undefined,
            ts.createFunctionTypeNode(
                undefined,
                [
                    ts.createParameter(
                        undefined,
                        undefined,
                        undefined,
                        ts.createIdentifier("state"),
                        undefined,
                        reducer.typeNode,
                        undefined
                    ),
                    ts.createParameter(
                        undefined,
                        undefined,
                        undefined,
                        ts.createIdentifier("action"),
                        undefined,
                        ts.createFunctionTypeNode(
                            undefined,
                            [],
                            ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)
                        ),
                        undefined
                    )
                ],
                reducer.typeNode
            ),
            undefined
        )
    })
}
