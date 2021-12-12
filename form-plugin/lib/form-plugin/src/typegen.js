"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    Object.defineProperty(o, k2, { enumerable: true, get: function() { return m[k]; } });
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || function (mod) {
    if (mod && mod.__esModule) return mod;
    var result = {};
    if (mod != null) for (var k in mod) if (k !== "default" && Object.prototype.hasOwnProperty.call(mod, k)) __createBinding(result, mod, k);
    __setModuleDefault(result, mod);
    return result;
};
Object.defineProperty(exports, "__esModule", { value: true });
const ts = __importStar(require("typescript"));
exports.default = {
    visitKeaProperty({ name, parsedLogic, node, getTypeNodeForNode, prepareForPrint }) {
        if (name === 'form') {
            let typeNode;
            if (ts.isArrowFunction(node) &&
                ts.isParenthesizedExpression(node.body) &&
                ts.isObjectLiteralExpression(node.body.expression)) {
                node = node.body.expression;
            }
            if (ts.isObjectLiteralExpression(node)) {
                const defaultProp = node.properties.find((prop) => prop.name.getText() === 'default');
                const defaultTypeNode = getTypeNodeForNode(defaultProp);
                typeNode = prepareForPrint(defaultTypeNode);
            }
            parsedLogic.reducers.push({
                name: 'form',
                typeNode: typeNode ||
                    ts.factory.createTypeReferenceNode(ts.factory.createIdentifier('Record'), [
                        ts.factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                        ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                    ]),
            });
            parsedLogic.extraInput['form'] = {
                withLogicFunction: true,
                typeNode: ts.factory.createTypeLiteralNode([
                    ts.factory.createPropertySignature(undefined, ts.factory.createIdentifier('default'), ts.factory.createToken(ts.SyntaxKind.QuestionToken), ts.factory.createTypeReferenceNode(ts.factory.createIdentifier('Record'), [
                        ts.factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                        ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                    ])),
                    ts.factory.createPropertySignature(undefined, ts.factory.createIdentifier('submit'), ts.factory.createToken(ts.SyntaxKind.QuestionToken), ts.factory.createFunctionTypeNode(undefined, [
                        ts.factory.createParameterDeclaration(undefined, undefined, undefined, ts.factory.createIdentifier('form'), undefined, typeNode ||
                            ts.factory.createTypeReferenceNode(ts.factory.createIdentifier('Record'), [
                                ts.factory.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                                ts.factory.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                            ]), undefined),
                    ], ts.factory.createKeywordTypeNode(ts.SyntaxKind.VoidKeyword))),
                ]),
            };
            parsedLogic.actions.push({
                name: 'submitForm',
                parameters: [],
                returnTypeNode: ts.factory.createTypeLiteralNode([
                    ts.factory.createPropertySignature(undefined, ts.factory.createIdentifier('value'), undefined, ts.factory.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword)),
                ]),
            });
        }
    },
};
//# sourceMappingURL=typegen.js.map