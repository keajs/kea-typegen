// https://dev.doctorevidence.com/how-to-write-a-typescript-transform-plugin-fc5308fdd943

import * as ts from 'typescript'
export default function(/*opts?: Opts*/) {
    function visitor(ctx: ts.TransformationContext, sf: ts.SourceFile) {
        const visitor: ts.Visitor = (node: ts.Node): ts.VisitResult => {
            // here we can check each node and potentially return
            // new nodes if we want to leave the node as is, and
            // continue searching through child nodes:
            return ts.visitEachChild(node, visitor, ctx)
        }
        return visitor
    }
    return (ctx: ts.TransformationContext): ts.Transformer => {
        return (sf: ts.SourceFile) => ts.visitNode(sf, visitor(ctx, sf))
    }
}
