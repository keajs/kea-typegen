package main

import (
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/checker"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
)

func literalMember(t *ast.Node, key string) *ast.Node {
	if t == nil || !ast.IsTypeLiteralNode(t) {
		return nil
	}
	for _, m := range t.Members() {
		if ast.IsPropertySignatureDeclaration(m) && ast.IsIdentifier(m.Name()) && m.Name().Text() == key {
			return m.Type()
		}
	}
	return nil
}

func propertyType(s *ast.Symbol) *ast.Node {
	if s != nil {
		if d := s.ValueDeclaration(); d != nil && ast.IsPropertySignatureDeclaration(d) {
			return d.Type()
		}
	}
	return nil
}

func (v *visitor) connect(t *checker.Type) {
	if s := t.Alias().Symbol(); s != nil && (s.Name() == "BuiltLogic" || s.Name() == "LogicWrapper") {
		return
	}
	for _, p := range v.c.GetPropertiesOfType(t) {
		list := initializer(p.ValueDeclaration())
		if list == nil || !ast.IsArrayLiteralExpression(list) {
			continue
		}
		for i := 0; i+1 < len(list.Elements()); i += 2 {
			ref, names := list.Elements()[i], list.Elements()[i+1]
			if !ast.IsArrayLiteralExpression(names) {
				continue
			}
			if ast.IsCallExpression(ref) {
				ref = ref.Expression()
			}
			source := scanner.GetTextOfNode(ref)
			other := v.c.GetTypeAtLocation(ref)
			field := "values"
			switch p.Name() {
			case "actions":
				field = "actionCreators"
			case "props", "values":
			default:
				continue
			}
			prop := v.c.GetPropertyOfType(other, field)
			if prop == nil {
				continue
			}
			group := v.c.GetTypeOfSymbolAtLocation(prop, ref)
			lookup := map[string]string{}
			var order []string
			for _, n := range names.Elements() {
				if !ast.IsStringLiteralLike(n) {
					continue
				}
				from, to, ok := strings.Cut(n.Text(), " as ")
				if !ok {
					to = from
				}
				if _, exists := lookup[from]; !exists {
					order = append(order, from)
				}
				lookup[from] = to
			}
			if field == "values" {
				for _, s := range v.c.GetPropertiesOfType(group) {
					if alias := lookup[s.Name()]; alias != "" {
						if typ := propertyType(s); typ != nil {
							v.gather(typ, nil)
							v.l.Selectors = append(v.l.Selectors, value{Name: alias, Type: scanner.GetTextOfNode(typ), Source: source})
						}
					}
				}
				continue
			}
			if decl := propertyType(prop); decl != nil && ast.IsTypeLiteralNode(decl) {
				for _, m := range decl.Members() {
					if !ast.IsPropertySignatureDeclaration(m) {
						continue
					}
					if alias := lookup[scanner.GetTextOfNode(m.Name())]; alias != "" {
						v.connectedAction(alias, source, m.Type(), nil)
					}
				}
			} else {
				for _, name := range order {
					s := v.c.GetPropertyOfType(group, name)
					if s == nil {
						continue
					}
					sig := v.signature(v.c.GetTypeOfSymbolAtLocation(s, ref))
					if sig == nil {
						continue
					}
					fn := v.c.SignatureToSignatureDeclaration(sig, ast.KindFunctionType, ref, printFlags)
					v.connectedAction(lookup[name], source, fn, ref)
				}
			}
		}
	}
}

func (v *visitor) connectedAction(name, source string, fn, loc *ast.Node) {
	if fn == nil || !ast.IsFunctionTypeNode(fn) {
		return
	}
	result := fn.Type()
	if ast.IsParenthesizedTypeNode(result) {
		result = result.Type()
	}
	payload := literalMember(result, "payload")
	if payload == nil {
		return
	}
	params := v.parameters(fn, loc, true)
	var typ string
	if loc == nil {
		typ = v.explicit(payload)
		v.gather(fn, nil)
	} else {
		typ = v.render(payload, loc, true)
		v.gather(fn, loc)
	}
	v.l.Actions = append(v.l.Actions, action{name, typ, source, params})
}

type externalAction struct{ result, payload string }

func (v *visitor) externalActions(obj *ast.Node) map[string]externalAction {
	actions := map[string]externalAction{}
	if obj == nil || !ast.IsObjectLiteralExpression(obj) {
		return actions
	}
	for _, p := range obj.Properties() {
		n := p.Name()
		if n == nil || !ast.IsComputedPropertyName(n) {
			continue
		}
		access := n.Expression()
		if !ast.IsPropertyAccessExpression(access) || !ast.IsPropertyAccessExpression(access.Expression()) {
			continue
		}
		ref := access.Expression().Expression()
		if ast.IsCallExpression(ref) {
			ref = ref.Expression()
		}
		group := v.c.GetPropertyOfType(v.c.GetTypeAtLocation(ref), "actionCreators")
		fn := literalMember(propertyType(group), access.Name().Text())
		if fn == nil || !ast.IsFunctionTypeNode(fn) {
			continue
		}
		symbol := v.c.GetSymbolAtLocation(n)
		if symbol == nil {
			continue
		}
		name := symbol.Name()
		result := fn.Type()
		if name == "__computed" {
			if lit := literalMember(result, "type"); lit != nil && ast.IsLiteralTypeNode(lit) {
				name = lit.AsLiteralTypeNode().Literal.Text()
			}
		}
		if sig := v.signature(v.c.GetTypeFromTypeNode(fn)); sig != nil {
			v.inferred(v.c.GetReturnTypeOfSignature(sig))
		}
		typ := v.printSource(fn, true)
		replaced := false
		for i := range v.l.ExtraActions {
			if v.l.ExtraActions[i].Name == name {
				v.l.ExtraActions[i].Type = typ
				replaced = true
				break
			}
		}
		if !replaced {
			v.l.ExtraActions = append(v.l.ExtraActions, value{Name: name, Type: typ})
		}
		if payload := literalMember(result, "payload"); payload != nil {
			actions[name] = externalAction{scanner.GetTextOfNode(result), scanner.GetTextOfNode(payload)}
		}
	}
	return actions
}
