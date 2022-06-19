import { VisitKeaPropertyArguments } from '../../src'
import * as ts from 'typescript'
import { factory } from 'typescript'

export function typedForm({ parsedLogic, node, getTypeNodeForNode, prepareForPrint }: VisitKeaPropertyArguments) {
    let typeNode: ts.TypeNode

    // extract `() => ({})` to just `{}`
    if (
        ts.isArrowFunction(node) &&
        ts.isParenthesizedExpression(node.body) &&
        ts.isObjectLiteralExpression(node.body.expression)
    ) {
        node = node.body.expression
    }

    // get type of `default` and prepare it for printing
    if (ts.isObjectLiteralExpression(node)) {
        const defaultProp = node.properties.find((prop) => prop.name.getText() === 'default')
        const defaultTypeNode = getTypeNodeForNode(defaultProp)
        // this gathers type references for auto-import and clones the node for accurate printing
        typeNode = prepareForPrint(defaultTypeNode)
    }

    // add reducer with this default type
    parsedLogic.reducers.push({
        name: 'form',
        typeNode:
            // the default given to us... or
            typeNode ||
            // ... Record<string, any>
            factory.createTypeReferenceNode(factory.createIdentifier('Record'), [
                factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
            ]),
    })

    // add action "submitForm" to parsedLogic
    parsedLogic.actions.push({
        name: 'submitForm',
        parameters: [],
        returnTypeNode: factory.createTypeLiteralNode([
            factory.createPropertySignature(
                undefined,
                factory.createIdentifier('value'),
                undefined,
                factory.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword),
            ),
        ]),
    })

    // add extra type for logic input (kea v2 style)
    parsedLogic.extraInput['form'] = {
        // adds support for both { form: (logic) => ({}) } and { form: {} }
        withLogicFunction: true,
        // type applied in LogicInput
        typeNode: factory.createTypeLiteralNode([
            // default?: Record<string, any>
            factory.createPropertySignature(
                undefined,
                factory.createIdentifier('default'),
                factory.createToken(ts.SyntaxKind.QuestionToken),
                factory.createTypeReferenceNode(factory.createIdentifier('Record'), [
                    factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                    factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                ]),
            ),
            // submit?: (form: $typeNode || Record<string, any>) => void
            factory.createPropertySignature(
                undefined,
                factory.createIdentifier('submit'),
                factory.createToken(ts.SyntaxKind.QuestionToken),
                factory.createFunctionTypeNode(
                    undefined,
                    [
                        factory.createParameterDeclaration(
                            undefined,
                            undefined,
                            undefined,
                            factory.createIdentifier('form'),
                            undefined,
                            typeNode ||
                                factory.createTypeReferenceNode(factory.createIdentifier('Record'), [
                                    factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                                    factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                                ]),
                            undefined,
                        ),
                    ],
                    factory.createKeywordTypeNode(ts.SyntaxKind.VoidKeyword),
                ),
            ),
        ]),
    }

    parsedLogic.extraLogicFields['custom'] = factory.createTypeLiteralNode([
        // default?: Record<string, any>
        factory.createPropertySignature(
            undefined,
            factory.createIdentifier('hello'),
            factory.createToken(ts.SyntaxKind.QuestionToken),
            factory.createLiteralTypeNode(factory.createStringLiteral('world')),
        ),
    ])
}
