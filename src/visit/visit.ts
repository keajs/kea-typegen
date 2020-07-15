import * as ts from 'typescript'
import * as path from 'path'
import { ParsedLogic } from '../types'
import { isKeaCall } from '../utils'
import { visitActions } from './visitActions'
import { visitReducers } from './visitReducers'
import { visitSelectors } from './visitSelectors'
import { visitLoaders } from './visitLoaders'

export function visitProgram(program: ts.Program, verbose: boolean = false): ParsedLogic[] {
    const checker = program.getTypeChecker()
    const parsedLogics: ParsedLogic[] = []

    for (const sourceFile of program.getSourceFiles()) {
        if (!sourceFile.isDeclarationFile && !sourceFile.fileName.endsWith('.type.ts')) {
            if (verbose) {
                console.log(`-> Visiting: ${path.relative(process.cwd(), sourceFile.fileName)}`)
            }
            ts.forEachChild(sourceFile, createVisit(checker, parsedLogics, sourceFile))
        }
    }

    return parsedLogics
}

export function createVisit(checker: ts.TypeChecker, parsedLogics: ParsedLogic[], sourceFile: ts.SourceFile) {
    return function visit(node: ts.Node) {
        if (!isKeaCall(node, checker)) {
            ts.forEachChild(node, visit)
            return
        }

        let logicName = 'logic'
        if (ts.isCallExpression(node.parent) && ts.isVariableDeclaration(node.parent.parent)) {
            logicName = node.parent.parent.name.getText()
        }

        const logicTypeName = `${logicName}Type`

        let logicTypeArguments = []

        const keaTypeArguments = ts.isCallExpression(node.parent) ? node.parent.typeArguments : []
        const keaTypeArgument = keaTypeArguments?.[0]

        // // kea<logicType>(..)
        if (keaTypeArgument?.typeName?.escapedText === logicTypeName) {
            // kea<logicType<somethingElse>>(...)
            // store <somethingElse> on the generated type!
            if (keaTypeArgument.typeArguments && keaTypeArgument.typeArguments.length > 0) {
                logicTypeArguments = (keaTypeArgument.typeArguments as ts.Node[]).map((a) => a.getFullText())
            }
        }

        const parsedLogic: ParsedLogic = {
            checker,
            logicName,
            logicTypeArguments: logicTypeArguments,
            fileName: sourceFile.fileName,
            actions: [],
            reducers: [],
            selectors: [],
        }

        const input = (node.parent as ts.CallExpression).arguments[0] as ts.ObjectLiteralExpression

        for (const inputProperty of input.properties) {
            const symbol = checker.getSymbolAtLocation(inputProperty.name as ts.Identifier)

            if (!symbol) {
                continue
            }

            const name = symbol.getName()
            let type = checker.getTypeOfSymbolAtLocation(symbol, symbol.valueDeclaration!)
            let typeNode = type ? checker.typeToTypeNode(type) : null

            if (typeNode && ts.isFunctionTypeNode(typeNode)) {
                type = type.getCallSignatures()[0].getReturnType()
            }

            if (name === 'actions') {
                visitActions(type, parsedLogic)
            } else if (name === 'reducers') {
                visitReducers(type, parsedLogic)
            } else if (name === 'selectors') {
                visitSelectors(type, parsedLogic)
            } else if (name === 'loaders') {
                visitLoaders(type, parsedLogic)
            }
        }

        parsedLogics.push(parsedLogic)
    }
}
