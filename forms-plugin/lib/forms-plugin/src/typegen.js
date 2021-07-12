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
                    ts.createTypeReferenceNode(ts.createIdentifier('Record'), [
                        ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                        ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                    ]),
            });
            parsedLogic.extraInput['form'] = {
                withLogicFunction: true,
                typeNode: ts.createTypeLiteralNode([
                    ts.createPropertySignature(undefined, ts.createIdentifier('default'), ts.createToken(ts.SyntaxKind.QuestionToken), ts.createTypeReferenceNode(ts.createIdentifier('Record'), [
                        ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                        ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                    ])),
                    ts.createPropertySignature(undefined, ts.createIdentifier('submit'), ts.createToken(ts.SyntaxKind.QuestionToken), ts.createFunctionTypeNode(undefined, [
                        ts.createParameter(undefined, undefined, undefined, ts.createIdentifier('form'), undefined, typeNode ||
                            ts.createTypeReferenceNode(ts.createIdentifier('Record'), [
                                ts.createKeywordTypeNode(ts.SyntaxKind.StringKeyword),
                                ts.createKeywordTypeNode(ts.SyntaxKind.AnyKeyword),
                            ]), undefined),
                    ], ts.createKeywordTypeNode(ts.SyntaxKind.VoidKeyword))),
                ]),
            };
            parsedLogic.actions.push({
                name: 'submitForm',
                parameters: [],
                returnTypeNode: ts.createTypeLiteralNode([
                    ts.createPropertySignature(undefined, ts.createIdentifier('value'), undefined, ts.createKeywordTypeNode(ts.SyntaxKind.BooleanKeyword), undefined),
                ]),
            });
        }
    },
};
//# sourceMappingURL=typegen.js.map