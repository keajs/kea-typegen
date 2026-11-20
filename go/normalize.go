package main

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// The published types have a stable order independent of checker discovery
// order. Match the existing printer's English locale comparison for names.
func (v *visitor) normalize(n *ast.Node) *ast.Node {
	if n == nil {
		return nil
	}
	var tr *ast.NodeVisitor
	tr = ast.NewNodeVisitor(func(n *ast.Node) *ast.Node {
		var list []*ast.Node
		switch n.Kind {
		case ast.KindTypeLiteral:
			list = n.Members()
		case ast.KindUnionType:
			list = n.AsUnionTypeNode().Types.Nodes
		}
		if len(list) > 1 && !slices.IsSortedFunc(list, v.compare) {
			list = slices.Clone(list)
			slices.SortStableFunc(list, v.compare)
			if ast.IsUnionTypeNode(n) {
				n = v.factory.NewUnionTypeNode(v.factory.NewNodeList(list))
			} else {
				n = v.factory.NewTypeLiteralNode(v.factory.NewNodeList(list))
			}
		}
		return tr.VisitEachChild(n)
	}, v.factory, ast.NodeVisitorHooks{})
	return tr.VisitNode(n)
}

func (v *visitor) compare(a, b *ast.Node) int {
	if a == nil || b == nil {
		if a == b {
			return 0
		}
		if a == nil {
			return -1
		}
		return 1
	}
	if a.Kind != b.Kind {
		if a.Kind == ast.KindUndefinedKeyword {
			return 1
		}
		if b.Kind == ast.KindUndefinedKeyword {
			return -1
		}
		return cmp.Compare(a.Kind, b.Kind)
	}
	switch a.Kind {
	case ast.KindLiteralType:
		x, y := a.AsLiteralTypeNode().Literal, b.AsLiteralTypeNode().Literal
		if ast.IsStringLiteralLike(x) && ast.IsStringLiteralLike(y) {
			return v.collator.CompareString(x.Text(), y.Text())
		}
		if ast.IsNumericLiteral(x) && ast.IsNumericLiteral(y) {
			nx, _ := strconv.ParseFloat(x.Text(), 64)
			ny, _ := strconv.ParseFloat(y.Text(), 64)
			return cmp.Compare(nx, ny)
		}
		return cmp.Compare(x.Kind, y.Kind)
	case ast.KindIdentifier:
		return v.collator.CompareString(a.Text(), b.Text())
	case ast.KindTypeReference:
		left, right := "", ""
		if x := a.AsTypeReferenceNode().TypeName; ast.IsIdentifier(x) {
			left = x.Text()
		}
		if x := b.AsTypeReferenceNode().TypeName; ast.IsIdentifier(x) {
			right = x.Text()
		}
		return v.collator.CompareString(left, right)
	case ast.KindPropertySignature:
		return v.collator.CompareString(v.printer.Emit(a.Name(), nil), v.printer.Emit(b.Name(), nil))
	case ast.KindImportType:
		x, y := a.AsImportTypeNode(), b.AsImportTypeNode()
		if d := v.compare(x.Argument, y.Argument); d != 0 {
			return d
		}
		return v.compare(x.Qualifier, y.Qualifier)
	case ast.KindArrayType:
		return v.compare(a.AsArrayTypeNode().ElementType, b.AsArrayTypeNode().ElementType)
	case ast.KindUnionType:
		return v.compareList(a.AsUnionTypeNode().Types.Nodes, b.AsUnionTypeNode().Types.Nodes)
	case ast.KindTypeLiteral:
		return v.compareList(a.Members(), b.Members())
	}
	return 0
}

func (v *visitor) compareList(a, b []*ast.Node) int {
	if d := cmp.Compare(len(a), len(b)); d != 0 {
		return d
	}
	for i, x := range a {
		if d := v.compare(x, b[i]); d != 0 {
			return d
		}
		if ast.IsPropertySignatureDeclaration(x) && ast.IsPropertySignatureDeclaration(b[i]) {
			if d := v.compare(x.Type(), b[i].Type()); d != 0 {
				return d
			}
		}
	}
	return 0
}

func newCollator() *collate.Collator { return collate.New(language.English) }
