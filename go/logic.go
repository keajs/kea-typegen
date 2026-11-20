package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/checker"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/nodebuilder"
	"github.com/microsoft/TypeScript/tsc/internal/printer"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
	"golang.org/x/text/collate"
)

type parameter struct {
	Name, Type string
	Optional   bool
}

type action struct {
	Name, Type, Source string
	Params             []parameter
}

type value struct {
	Name, Type, Source string
	Inputs             []parameter
}

type listener struct{ Name, Payload, Action string }
type annotation struct {
	End               int
	Type, SourceValue string
}

type importedName struct{ File, Name string }
type step struct {
	Name  string
	Index int
}

type logic struct {
	Steps                                []step
	Name, File, ImportedFrom, Key, Props string
	End                                  int
	BuilderArray, HasPath, PropsLiteral  bool
	Path, TypeArgs, Events               []string
	Actions                              []action
	Reducers, Selectors                  []value
	Listeners, SharedListeners           []listener
	Annotations                          []annotation
	Imports                              []importedName
	ExtraActions                         []value
}

type visitor struct {
	c             *checker.Checker
	program       *compiler.Program
	file          *ast.SourceFile
	opts          options
	goOnlyPlugin  string
	sidecars      map[string]*sidecar
	host          *pluginHost
	globalPlugins []*sidecar
	customGlobals bool
	err           error
	l             *logic
	imported      map[string]bool
	factory       *ast.NodeFactory
	collator      *collate.Collator
	printer       *printer.Printer
}

var extension = regexp.MustCompile(`\.[cm]?[jt]sx?$`)

func (v *visitor) visitFile() ([]*logic, bool) {
	var logics []*logic
	plugin := false
	v.collator = newCollator()
	v.factory = ast.NewNodeFactory(ast.NodeFactoryHooks{})
	v.printer = printer.NewPrinter(printer.PrinterOptions{RemoveComments: true}, printer.PrintHandlers{}, nil)
	walk(v.file.AsNode(), func(n *ast.Node) bool {
		if v.err != nil || !ast.IsCallExpression(n) {
			return v.err != nil
		}
		id := n.Expression()
		if !ast.IsIdentifier(id) || id.Text() != "kea" {
			return false
		}
		arg := first(n.Arguments())
		if arg == nil || !(ast.IsArrayLiteralExpression(arg) || ast.IsObjectLiteralExpression(arg)) {
			return false
		}
		if s := v.c.GetSymbolAtLocation(id); s == nil || s.Name() != "kea" {
			return false
		}
		name := "logic"
		if ast.IsVariableDeclaration(n.Parent) {
			name = scanner.GetTextOfNode(n.Parent.Name())
		}
		l := &logic{Name: name, File: v.file.FileName().AsString(), End: v.end(id), BuilderArray: ast.IsArrayLiteralExpression(arg)}
		if t := first(n.TypeArguments()); t != nil {
			if !ast.IsTypeReferenceNode(t) {
				return false
			}
			ref := t.AsTypeReferenceNode().TypeName
			if !ast.IsIdentifier(ref) || ref.Text() != name+"Type" {
				return false
			}
			d := declaration(v.c.GetSymbolAtLocation(ref))
			if d != nil {
				if ast.IsImportSpecifier(d) {
					l.ImportedFrom = moduleFile(v.c, d)
				} else if !v.managedDeclaration(d, name) {
					return false
				}
			}
			for _, a := range t.TypeArguments() {
				l.TypeArgs = append(l.TypeArgs, scanner.GetTextOfNode(a))
			}
		}
		relative, _ := filepath.Rel(v.opts.RootPath, l.File)
		l.Path = strings.Split(strings.ReplaceAll(extension.ReplaceAllString(filepath.ToSlash(relative), ""), "/", "."), ".")
		v.l = l
		v.imported = map[string]bool{}
		if l.BuilderArray {
			for i, call := range arg.Elements() {
				if !ast.IsCallExpression(call) || len(call.Arguments()) == 0 {
					continue
				}
				builderName := scanner.GetTextOfNode(call.Expression())
				var sc *sidecar
				if file := importedModule(v.c, call.Expression()); file != "" {
					var found bool
					if sc, found = v.sidecars[file]; !found {
						sc = v.host.load(file, false)
						v.sidecars[file] = sc
					}
				}
				if !plugin {
					v.builder(builderName, call.Arguments()[0], nil)
				}
				if !v.pluginsFor(builderName, sc, call.Arguments()[0], plugin) {
					plugin = true
				}
				l.Steps = append(l.Steps, step{builderName, i})
			}
		} else {
			for i, p := range arg.Properties() {
				if !ast.IsPropertyAssignment(p) {
					continue
				}
				if sym := v.c.GetSymbolAtLocation(p.Name()); sym != nil {
					if !plugin {
						v.builder(sym.Name(), p.Initializer(), sym)
					}
					if !v.pluginsFor(sym.Name(), nil, p.Initializer(), plugin) {
						plugin = true
					}
					l.Steps = append(l.Steps, step{sym.Name(), i})
				}
			}
		}
		logics = append(logics, l)
		return false // nested kea calls are independent logics
	})
	if plugin && v.goOnlyPlugin != "" && v.err == nil {
		v.err = fmt.Errorf("%s needs a companion JavaScript visitor to share %s with a JavaScript plugin", v.goOnlyPlugin, v.file.FileName().AsString())
	}
	return logics, plugin
}

func (v *visitor) managedDeclaration(d *ast.Node, name string) bool {
	stmts := v.file.Statements.Nodes
	for i := slices.Index(stmts, d); i >= 0; i-- {
		n := stmts[i]
		if !(ast.IsTypeAliasDeclaration(n) || ast.IsInterfaceDeclaration(n)) {
			break
		}
		if n != d && !slices.Contains([]string{name + "Values", name + "Actions", name + "Props", name + "Meta"}, n.Name().Text()) {
			break
		}
		for r := range scanner.GetLeadingCommentRanges(v.factory, v.file.Text(), n.Pos()) {
			if strings.Contains(v.file.Text()[r.Pos():r.End()], "kea-typegen") {
				return true
			}
		}
	}
	return false
}

func (v *visitor) end(n *ast.Node) int { return v.file.GetPositionMap().UTF8ToUTF16(n.End()) }
func (v *visitor) signature(t *checker.Type) *checker.Signature {
	return first(v.c.GetSignaturesOfType(t, checker.SignatureKindCall))
}

const printFlags = nodebuilder.FlagsNoTruncation | nodebuilder.FlagsIgnoreErrors

func (v *visitor) typeNode(t *checker.Type, location *ast.Node) *ast.Node {
	return v.c.TypeToTypeNode(t, location, printFlags, nil)
}

func (v *visitor) inferred(t *checker.Type) string {
	n := v.typeNode(t, v.file.AsNode())
	return v.render(n, v.file.AsNode(), false)
}

func (v *visitor) explicit(n *ast.Node) string {
	if n == nil {
		return "any"
	}
	v.gather(n, nil)
	return v.printSource(n, true)
}

func (v *visitor) printSource(n *ast.Node, sort bool) string {
	if n == nil {
		return "any"
	}
	file := ast.GetSourceFileOfNode(n)
	if sort {
		n = v.normalize(n)
	}
	return printer.NewPrinter(printer.PrinterOptions{}, printer.PrintHandlers{}, nil).Emit(n, file)
}

func (v *visitor) defaultType(n *ast.Node, unwrap bool) string {
	if n == nil {
		return "any"
	}
	switch n.Kind {
	case ast.KindAsExpression:
		t := n.Type()
		if ast.IsParenthesizedTypeNode(t) {
			t = t.Type()
		}
		if unwrap && ast.IsFunctionTypeNode(t) {
			t = t.Type()
		}
		return v.explicit(t)
	case ast.KindTrueKeyword, ast.KindFalseKeyword:
		return "boolean"
	case ast.KindStringLiteral, ast.KindNoSubstitutionTemplateLiteral:
		return "string"
	case ast.KindNumericLiteral:
		return "number"
	case ast.KindArrayLiteralExpression:
		if len(n.Elements()) == 0 {
			return "any[]"
		}
	}
	t := v.typeNode(v.c.GetTypeAtLocation(n), v.file.AsNode())
	if unwrap && t != nil && ast.IsFunctionTypeNode(t) {
		t = t.Type()
	}
	return v.render(t, v.file.AsNode(), false)
}

func rootName(n *ast.Node) string {
	for ast.IsQualifiedName(n) {
		n = n.AsQualifiedName().Left
	}
	return n.Text()
}

func packageName(path string) string {
	path = filepath.ToSlash(path)
	if i := strings.LastIndex(path, "/node_modules/"); i >= 0 {
		path = path[i+len("/node_modules/"):]
	}
	if path == "" || strings.ContainsAny(path[:1], "./~") || strings.HasPrefix(path, "@types/") {
		return ""
	}
	parts := strings.Split(path, "/")
	if strings.HasPrefix(path, "@") {
		if len(parts) < 2 {
			return ""
		}
		return strings.Join(parts[:2], "/")
	}
	return parts[0]
}

func (v *visitor) addImport(file, name string) {
	if v.imported[name] || file == "" || name == "" || file == "typescript" || strings.Contains(file, "/node_modules/typescript/") || strings.HasPrefix(file, "typescript/") {
		return
	}
	if pkg := packageName(file); pkg != "" {
		for _, stmt := range v.file.Statements.Nodes {
			if !ast.IsImportDeclaration(stmt) {
				continue
			}
			spec := stmt.ModuleSpecifier()
			if packageName(spec.Text()) != pkg {
				continue
			}
			if sym := v.c.GetSymbolAtLocation(spec); sym != nil {
				for _, exp := range v.c.GetExportsOfModule(sym) {
					if exp.Name() == name {
						file = spec.Text()
						break
					}
				}
				if file == spec.Text() {
					break
				}
			}
		}
	}
	v.l.Imports = append(v.l.Imports, importedName{file, name})
	v.imported[name] = true
}

func (v *visitor) gather(n, location *ast.Node) {
	walk(n, func(n *ast.Node) bool {
		if !ast.IsTypeReferenceNode(n) {
			return false
		}
		name := n.AsTypeReferenceNode().TypeName
		root := rootName(name)
		if v.imported[root] {
			return false
		}
		var sym *ast.Symbol
		if location != nil {
			sym = v.c.ResolveName(root, location, ast.SymbolFlagsType|ast.SymbolFlagsNamespace|ast.SymbolFlagsAlias, false)
		} else {
			sym = v.c.GetSymbolAtLocation(name)
		}
		d := declaration(sym)
		if d == nil || v.program.IsSourceFileDefaultLibrary(ast.GetSourceFileOfNode(d).PathKey()) {
			return false
		}
		if ast.IsQualifiedName(name) || ast.IsImportSpecifier(d) || ast.IsTypeAliasDeclaration(d) || ast.IsInterfaceDeclaration(d) || ast.IsEnumDeclaration(d) || ast.IsClassDeclaration(d) {
			v.addImport(moduleFile(v.c, d), root)
		}
		return false
	})
}

func (v *visitor) render(n, location *ast.Node, resolve bool) string {
	if n == nil {
		return "any"
	}
	v.gather(n, location)
	// Selector inputs sort while their import() qualifications are still present.
	// Connected action signatures resolve imports before sorting their members.
	sortAfter := resolve && location != nil && !ast.IsFunctionLike(location)
	if !sortAfter {
		n = v.normalize(n)
	}
	if resolve {
		var tr *ast.NodeVisitor
		tr = ast.NewNodeVisitor(func(n *ast.Node) *ast.Node {
			if ast.IsImportTypeNode(n) {
				imp := n.AsImportTypeNode()
				if !imp.IsTypeOf && imp.Qualifier != nil && ast.IsLiteralTypeNode(imp.Argument) {
					file := imp.Argument.AsLiteralTypeNode().Literal.Text()
					if strings.HasPrefix(file, ".") {
						file = filepath.Join(filepath.Dir(v.l.File), file)
					}
					v.addImport(file, rootName(imp.Qualifier))
					return v.factory.NewTypeReferenceNode(imp.Qualifier, tr.VisitNodes(imp.TypeArguments))
				}
			}
			return tr.VisitEachChild(n)
		}, v.factory, ast.NodeVisitorHooks{})
		n = tr.VisitNode(n)
	}
	if sortAfter {
		n = v.normalize(n)
	}
	return v.printer.Emit(n, v.file)
}
