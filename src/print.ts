import * as ts from 'typescript'
import { ParsedLogic } from './types'

export function logicToTypeString(parsedLogic: ParsedLogic) {
    const logicType = createLogicType(parsedLogic)
    const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed })
    const sourceFile = ts.createSourceFile(
        'logic.ts',
        '',
        ts.ScriptTarget.Latest,
        false,
        ts.ScriptKind.TS,
    )
    return printer.printNode(ts.EmitHint.Unspecified, logicType, sourceFile)
}

export function createLogicType(parsedLogic: ParsedLogic) {
    const createProperty = (name, typeNode) =>
        ts.createPropertySignature(undefined, ts.createIdentifier(name), undefined, typeNode, undefined)

    return ts.createInterfaceDeclaration(
        undefined,
        [ts.createModifier(ts.SyntaxKind.ExportKeyword)],
        ts.createIdentifier(`${parsedLogic.logicName}Type`),
        undefined,
        undefined,
        [
            createProperty('actionCreators', ts.createTypeLiteralNode(createActions(parsedLogic))),
            // actionKeys
            createProperty('actions', ts.createTypeLiteralNode(createActions(parsedLogic))),
            // build
            // cache
            // connections
            // constants
            // defaults
            // events
            // extend
            // inputs
            // listeners
            // mount
            // path
            // pathString
            // propTypes
            // props
            createProperty('reducer', createReducer(parsedLogic)),
            // reducerOptions
            createProperty('reducers', ts.createTypeLiteralNode(createReducers(parsedLogic))),
            createProperty('selector', createSelector(parsedLogic)),
            createProperty('selectors', ts.createTypeLiteralNode(createSelectors(parsedLogic))),
            // sharedListeners
            createProperty('values', ts.createTypeLiteralNode(createValues(parsedLogic))),
            // wrap
            // _isKea
            // _isKeaWithKey
        ],
    )
}

function createActions(parsedLogic: ParsedLogic) {
    return parsedLogic.actions.map((action) => {
        const returnType = action.signature.getReturnType()
        const parameters = action.signature.getDeclaration().parameters

        return ts.createPropertySignature(
            undefined,
            ts.createIdentifier(action.name),
            undefined,
            ts.createFunctionTypeNode(
                undefined,
                parameters,
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
                        ts.createIdentifier('state'),
                        undefined,
                        reducer.typeNode,
                        undefined,
                    ),
                    ts.createParameter(
                        undefined,
                        undefined,
                        undefined,
                        ts.createIdentifier('action'),
                        undefined,
                        ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        undefined,
                    ),
                    ts.createParameter(
                        undefined,
                        undefined,
                        undefined,
                        ts.createIdentifier('fullState'),
                        undefined,
                        ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        undefined,
                    ),
                ],
                reducer.typeNode,
            ),
            undefined,
        )
    })
}

function createReducer(parsedLogic: ParsedLogic) {
    return ts.createFunctionTypeNode(
        undefined,
        [
            ts.createParameter(
                undefined,
                undefined,
                undefined,
                ts.createIdentifier('state'),
                undefined,
                ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                undefined,
            ),
            ts.createParameter(
                undefined,
                undefined,
                undefined,
                ts.createIdentifier('action'),
                undefined,
                ts.createFunctionTypeNode(undefined, [], ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword)),
                undefined,
            ),
            ts.createParameter(
                undefined,
                undefined,
                undefined,
                ts.createIdentifier('fullState'),
                undefined,
                ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                undefined,
            ),
        ],
        ts.createTypeLiteralNode(
            parsedLogic.reducers.map((reducer) =>
                ts.createPropertySignature(
                    undefined,
                    ts.createIdentifier(reducer.name),
                    undefined,
                    reducer.typeNode,
                    undefined,
                ),
            ),
        ),
    )
}

function createSelector(parsedLogic: ParsedLogic) {
    return ts.createFunctionTypeNode(
        undefined,
        [
            ts.createParameter(
                undefined,
                undefined,
                undefined,
                ts.createIdentifier('state'),
                undefined,
                ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                undefined,
            ),
        ],
        ts.createTypeLiteralNode(
            parsedLogic.reducers.map((reducer) =>
                ts.createPropertySignature(
                    undefined,
                    ts.createIdentifier(reducer.name),
                    undefined,
                    reducer.typeNode,
                    undefined,
                ),
            ),
        ),
    )
}

function createSelectors(parsedLogic: ParsedLogic) {
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
                        ts.createIdentifier('state'),
                        undefined,
                        ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        undefined,
                    ),
                    ts.createParameter(
                        undefined,
                        undefined,
                        undefined,
                        ts.createIdentifier('props'),
                        undefined,
                        ts.createTypeReferenceNode(ts.createIdentifier('Record'), [
                            ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                            ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                        ]),
                        undefined,
                    ),
                ],
                reducer.typeNode,
            ),
            undefined,
        )
    })
}

function createValues(parsedLogic: ParsedLogic) {
    return parsedLogic.reducers.map((reducer) => {
        return ts.createPropertySignature(
            undefined,
            ts.createIdentifier(reducer.name),
            undefined,
            reducer.typeNode,
            undefined,
        )
    })
}
