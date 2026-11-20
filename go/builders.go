package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/checker"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
)

func body(n *ast.Node) *ast.Node {
	if ast.IsFunctionLike(n) {
		n = n.Body()
	}
	if n != nil {
		n = ast.SkipParentheses(n)
	}
	return n
}

func initializer(d *ast.Node) *ast.Node {
	if d != nil && (ast.IsPropertyAssignment(d) || ast.IsPropertyDeclaration(d) || ast.IsPropertySignatureDeclaration(d)) {
		return d.Initializer()
	}
	return nil
}

func element(n *ast.Node, i int) *ast.Node {
	if n != nil && ast.IsArrayLiteralExpression(n) && len(n.Elements()) > i {
		return n.Elements()[i]
	}
	return nil
}

func (v *visitor) builder(name string, expr *ast.Node, property *ast.Symbol) {
	l := v.l
	switch name {
	case "path", "logicPath":
		l.HasPath = true
		if n := body(expr); name == "path" && n != nil && ast.IsArrayLiteralExpression(n) {
			l.Path = []string{}
			for _, e := range n.Elements() {
				s := "*"
				if ast.IsStringLiteral(e) {
					s = e.Text()
				}
				l.Path = append(l.Path, s)
			}
		}
		return
	case "props":
		l.Props = v.defaultType(expr, false)
		if ast.IsAsExpression(expr) {
			n := expr.Type()
			if ast.IsParenthesizedTypeNode(n) {
				n = n.Type()
			}
			l.PropsLiteral = ast.IsTypeLiteralNode(n)
		} else if strings.HasPrefix(l.Props, "{") {
			l.PropsLiteral = ast.IsTypeLiteralNode(v.typeNode(v.c.GetTypeAtLocation(expr), v.file.AsNode()))
		}
		return
	case "key", "connect", "actions", "defaults", "reducers", "selectors", "listeners", "sharedListeners", "events", "windowValues", "loaders", "lazyLoaders":
	default:
		return
	}
	var t *checker.Type
	if property == nil {
		t = v.c.GetTypeAtLocation(expr)
	} else {
		t = v.c.GetTypeOfSymbolAtLocation(property, property.ValueDeclaration())
	}
	if sig := v.signature(t); sig != nil && len(v.c.GetPropertiesOfType(t)) == 0 {
		t = v.c.GetReturnTypeOfSignature(sig)
	}
	switch name {
	case "key":
		l.Key = v.inferred(t)
		return
	case "connect":
		v.connect(t)
		return
	}
	var extra map[string]externalAction
	if name == "listeners" {
		extra = v.externalActions(body(expr))
	}
	for _, prop := range v.c.GetPropertiesOfType(t) {
		k := prop.Name()
		d := prop.ValueDeclaration()
		n := initializer(d)
		switch name {
		case "actions":
			if n != nil {
				a := action{Name: k, Type: "{ value: true }"}
				if ast.IsFunctionLike(n) {
					a.Params = v.parameters(n, nil, false)
					a.Type = v.returnType(n)
				}
				l.Actions = append(l.Actions, a)
			}
		case "defaults":
			l.Reducers = append(l.Reducers, value{Name: k, Type: v.defaultType(n, false)})
		case "reducers":
			if n != nil {
				typ := ""
				if ast.IsArrayLiteralExpression(n) {
					typ = v.defaultType(element(n, 0), true)
					if elems := n.Elements(); len(elems) > 1 {
						v.externalActions(elems[len(elems)-1])
					}
				} else if ast.IsObjectLiteralExpression(n) {
					typ = "any"
				}
				l.Reducers = append(l.Reducers, value{Name: k, Type: typ})
			}
		case "loaders", "lazyLoaders":
			v.loader(k, n)
		case "selectors":
			v.selector(k, n)
		case "windowValues":
			if n != nil && ast.IsFunctionLike(n) {
				if sig := v.signature(v.c.GetTypeOfSymbolAtLocation(prop, n)); sig != nil {
					l.Reducers = append(l.Reducers, value{Name: k, Type: v.inferred(v.c.GetReturnTypeOfSignature(sig))})
				}
			}
		case "events":
			l.Events = append(l.Events, k)
		case "sharedListeners":
			if n != nil && ast.IsFunctionLike(n) {
				payload := "any"
				if p := first(n.Parameters()); p != nil {
					payload = v.inferred(v.c.GetTypeAtLocation(p))
				}
				l.SharedListeners = append(l.SharedListeners, listener{k, payload, "{ type: string; payload: " + payload + " }"})
			}
		case "listeners":
			if a, ok := extra[k]; ok {
				l.Listeners = append(l.Listeners, listener{k, a.payload, a.result})
			} else if a := v.action(k); a != nil {
				literal, _ := json.Marshal(strings.TrimPrefix(upperLetter.ReplaceAllStringFunc(k, spaceLetter), " ") + " (" + strings.Join(l.Path, ".") + ")")
				l.Listeners = append(l.Listeners, listener{k, a.Type, "{ type: " + string(literal) + "; payload: " + a.Type + " }"})
			}
		}
	}
}

var upperLetter = regexp.MustCompile(`\.?[A-Z]`)

func spaceLetter(s string) string { return " " + strings.ToLower(strings.TrimPrefix(s, ".")) }
func (v *visitor) action(name string) *action {
	for i := range v.l.Actions {
		if v.l.Actions[i].Name == name {
			return &v.l.Actions[i]
		}
	}
	return nil
}

func (v *visitor) returnType(fn *ast.Node) string {
	if t := fn.Type(); t != nil {
		return v.explicit(t)
	}
	sig := v.c.GetSignatureFromDeclaration(fn)
	if sig == nil {
		return "any"
	}
	return v.inferred(v.c.GetReturnTypeOfSignature(sig))
}

func (v *visitor) parameters(fn, location *ast.Node, resolve bool) []parameter {
	var params []parameter
	for _, p := range fn.Parameters() {
		params = append(params, v.parameter(p, location, resolve))
	}
	return params
}

func (v *visitor) parameter(p, location *ast.Node, resolve bool) parameter {
	name := p.Name()
	var text, typ string
	if location == nil {
		text = scanner.GetTextOfNode(name)
		typ = v.explicit(p.Type())
	} else {
		text = v.printer.Emit(name, v.file)
		typ = v.render(p.Type(), location, resolve)
	}
	return parameter{text, typ, p.QuestionToken() != nil || p.Initializer() != nil}
}

func (v *visitor) selector(k string, n *ast.Node) {
	input, fn := element(n, 0), element(n, 1)
	if fn == nil || !ast.IsFunctionLike(fn) {
		return
	}
	s := value{Name: k}
	var names []string
	if ast.IsFunctionLike(input) {
		if b := input.Body(); b != nil && ast.IsArrayLiteralExpression(b) {
			for _, e := range b.Elements() {
				name := ""
				if ast.IsPropertyAccessExpression(e) {
					name = scanner.GetTextOfNode(e.Name())
				}
				names = append(names, name)
			}
		}
	}
	if sig := v.signature(v.c.GetTypeAtLocation(input)); sig != nil {
		tuple := v.typeNode(v.c.GetReturnTypeOfSignature(sig), input)
		if tuple != nil && ast.IsTupleTypeNode(tuple) {
			taken := map[string]int{}
			for i, e := range tuple.AsTupleTypeNode().Elements.Nodes {
				name := "arg"
				source := ""
				if i < len(names) && names[i] != "" {
					source = names[i]
					name = source
				}
				taken[name]++
				if taken[name] > 1 {
					name += strconv.Itoa(taken[name])
				}
				typ := "any"
				if ast.IsFunctionTypeNode(e) {
					typ = v.render(e.Type(), input, true)
				}
				s.Inputs = append(s.Inputs, parameter{Name: name, Type: typ})
				if i < len(fn.Parameters()) && typ != "any" {
					p := fn.Parameters()[i]
					if p.Type() == nil && p.AsParameterDeclaration().DotDotDotToken == nil && ast.IsIdentifier(p.Name()) {
						v.l.Annotations = append(v.l.Annotations, annotation{v.end(p), typ, source})
					}
				}
			}
		}
	}
	s.Type = v.returnType(fn)
	v.l.Selectors = append(v.l.Selectors, s)
}

func (v *visitor) loader(k string, n *ast.Node) {
	var def, obj *ast.Node
	if n != nil && ast.IsObjectLiteralExpression(n) {
		obj = n
		for _, p := range n.Properties() {
			if p.Name() != nil && scanner.GetTextOfNode(p.Name()) == "__default" {
				def = initializer(p)
			}
		}
	} else {
		def = element(n, 0)
		obj = element(n, 1)
	}
	typ := v.defaultType(def, false)
	v.l.Reducers = append(v.l.Reducers, value{Name: k, Type: typ}, value{Name: k + "Loading", Type: "boolean"})
	if obj == nil || !ast.IsObjectLiteralExpression(obj) {
		return
	}
	for _, p := range obj.Properties() {
		fn := initializer(p)
		if fn == nil || !ast.IsFunctionLike(fn) {
			continue
		}
		sym := v.c.GetSymbolAtLocation(p.Name())
		if sym == nil {
			continue
		}
		name := sym.Name()
		if name == "__default" {
			continue
		}
		base := v.action(name)
		if base == nil {
			a := action{Name: name, Type: "any"}
			if param := first(fn.Parameters()); param != nil {
				arg := v.parameter(param, nil, false)
				a.Params = []parameter{arg}
				a.Type = v.printSource(param.Type(), false)
			}
			v.l.Actions = append(v.l.Actions, a)
			base = &v.l.Actions[len(v.l.Actions)-1]
		}
		if v.action(name+"Success") == nil {
			result := typ
			if sig := v.signature(v.c.GetTypeAtLocation(fn)); sig != nil {
				t := unpromise(v.typeNode(v.c.GetReturnTypeOfSignature(sig), v.file.AsNode()))
				if !emptyType(t) {
					result = v.render(t, v.file.AsNode(), false)
				} else if t = unpromise(fn.Type()); !emptyType(t) {
					result = v.explicit(t)
				}
			}
			v.l.Actions = append(v.l.Actions, action{Name: name + "Success", Type: fmt.Sprintf("{ %s: %s; payload?: %s }", k, result, base.Type), Params: []parameter{{k, result, false}, {"payload", base.Type, true}}})
		}
		if v.action(name+"Failure") == nil {
			v.l.Actions = append(v.l.Actions, action{Name: name + "Failure", Type: "{ error: string; errorObject?: any }", Params: []parameter{{"error", "string", false}, {"errorObject", "any", true}}})
		}
	}
}

func unpromise(t *ast.Node) *ast.Node {
	if t != nil && ast.IsTypeReferenceNode(t) && rootName(t.AsTypeReferenceNode().TypeName) == "Promise" {
		return first(t.TypeArguments())
	}
	return t
}

func emptyType(t *ast.Node) bool {
	return t == nil || t.Kind == ast.KindAnyKeyword || t.Kind == ast.KindUnknownKeyword || ast.IsTypeLiteralNode(t) && len(t.Members()) == 0
}
