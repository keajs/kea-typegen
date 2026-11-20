// The CLI owns filesystem edits and JavaScript plugin loading. This worker owns
// the compiler program for one immutable generation pass.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/checker"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnosticwriter"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/cachedvfs"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/osvfs"
)

type options struct {
	TsConfigPath, SourceFilePath, RootPath string
	ShowTsErrors, Inline                   bool
	InlinePaths                            []string
}

type projectResult struct {
	Files, ESM, Types, PluginFiles, PluginContexts []string
	Logics                                         []generated
	Diagnostics                                    string
	CustomGlobalHooks                              bool
}

var keaCall = regexp.MustCompile(`\bkea\s*[<(]`)
var jsFile = regexp.MustCompile(`(?i)\.[cm]?jsx?$`)

func analyze(opts options) (*projectResult, error) {
	ctx := context.Background()
	fs := cachedvfs.From(bundled.WrapFS(osvfs.FS()))
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	base := tspath.RootedDirectoryPathFromAbsolute(cwd)
	co := &core.CompilerOptions{NoEmit: core.TSTrue}
	var config *tsoptions.ParsedCommandLine
	var diagnostics []*ast.Diagnostic
	if opts.TsConfigPath != "" {
		config, diagnostics = tsoptions.GetParsedCommandLineOfConfigFile(tspath.RootedFilePathFromAbsolute(opts.TsConfigPath), co, nil, fs, nil)
		if config != nil {
			diagnostics = append(diagnostics, config.Errors...)
		}
	} else {
		config = tsoptions.NewParsedCommandLine(co, nil, nil, base, fs.CaseSensitivity())
	}
	format := func(ds []*ast.Diagnostic) string {
		var out strings.Builder
		diagnosticwriter.FormatDiagnosticsWithColorAndContext(&out, diagnosticwriter.FromASTDiagnostics(ds), &diagnosticwriter.FormattingOptions{CurrentDirectory: base, CaseSensitivity: fs.CaseSensitivity(), NewLine: "\n"})
		return out.String()
	}
	for _, d := range diagnostics {
		if opts.SourceFilePath == "" || d.Code() != 18003 {
			return nil, fmt.Errorf("%s", format(diagnostics))
		}
	}
	if opts.SourceFilePath != "" {
		config = config.WithFileNames([]tspath.RootedFilePath{tspath.RootedFilePathFromAbsolute(opts.SourceFilePath)})
		if jsFile.MatchString(opts.SourceFilePath) {
			config.ParsedConfig.CompilerOptions.AllowJs = core.TSTrue
		}
	}
	p := compiler.NewProgram(compiler.ProgramOptions{
		ProgramConfig: compiler.ProgramConfig{Config: config},
		ProgramHosts:  compiler.ProgramHosts{Host: compiler.NewCompilerHost(fs, bundled.LibPath(), nil, nil, nil)},
	})
	if opts.SourceFilePath != "" && p.GetSourceFile(tspath.RootedFilePathFromAbsolute(opts.SourceFilePath)) == nil {
		return nil, fmt.Errorf("source file is missing from the TypeScript program: %s", opts.SourceFilePath)
	}
	p.BindSourceFiles()
	result := &projectResult{Types: p.Options().Types}
	if opts.ShowTsErrors {
		ds := p.GetProgramDiagnostics()
		ds = append(ds, p.GetSyntacticDiagnostics(ctx, nil)...)
		ds = append(ds, p.GetSemanticDiagnostics(ctx, nil)...)
		result.Diagnostics = format(ds)
	}
	var candidates []*ast.SourceFile
	c, done := p.GetTypeChecker(ctx)
	defer done()
	ext := p.Options().GetModuleResolutionKind()
	host := &pluginHost{files: make(map[string]*sidecar)}
	defer host.close()
	globalHooks := make(map[string]*sidecar)
	var orderedHooks []*sidecar
	noPrefilter := os.Getenv("KEA_TYPEGEN_NO_PREFILTER") == "1"
	metaArgument, foundMeta := true, false
	for _, f := range p.SourceFiles() {
		name := f.FileName().AsString()
		if bundled.IsBundled(name) {
			continue
		}
		result.Files = append(result.Files, name)
		if (ext == core.ModuleResolutionKindNode16 || ext == core.ModuleResolutionKindNodeNext) && p.GetImpliedNodeFormatForEmit(f) == core.ModuleKindESNext {
			result.ESM = append(result.ESM, name)
		}
		if !foundMeta && f.IsDeclarationFile && strings.Contains(name, "/kea/") {
			for _, s := range f.Statements.Nodes {
				if (ast.IsInterfaceDeclaration(s) || ast.IsTypeAliasDeclaration(s)) && s.Name().Text() == "MakeLogicType" {
					metaArgument = s.TypeParameterList() != nil && len(s.TypeParameterList().Nodes) >= 4
					foundMeta = true
					break
				}
			}
		}
		if f.IsDeclarationFile || strings.HasSuffix(name, "Type.ts") {
			continue
		}
		if strings.Contains(f.Text(), "resetContext") && collectGlobalPlugins(f, c, host, globalHooks, &orderedHooks) {
			result.PluginContexts = append(result.PluginContexts, name)
		}
		if (opts.SourceFilePath == "" || filepath.Clean(name) == filepath.Clean(opts.SourceFilePath)) && (noPrefilter || keaCall.MatchString(f.Text())) {
			candidates = append(candidates, f)
		}
	}

	// The compiler partitions related source files across exclusive checkers.
	// Keep type objects on their owning checker and merge outputs in file order.
	for _, sc := range orderedHooks {
		if sc.err != nil {
			return nil, fmt.Errorf("%s: %w", sc.path, sc.err)
		}
		if sc.manifest == nil {
			result.CustomGlobalHooks = true
		}
	}
	if result.CustomGlobalHooks {
		for _, sc := range orderedHooks {
			if !sc.js {
				return nil, fmt.Errorf("%s needs a JavaScript visitor alongside custom global JavaScript hooks", sc.path)
			}
		}
	}
	groups := make(map[*checker.Checker][]int)
	for index, file := range candidates {
		owner, release := p.GetTypeCheckerForFile(ctx, file)
		groups[owner] = append(groups[owner], index)
		release()
	}
	outputs := make([][]generated, len(candidates))
	pluginFile := make([]bool, len(candidates))
	visitErrors := make([]error, len(candidates))
	p.ForEachCheckerParallel(func(_ int, owner *checker.Checker) {
		paths := make(map[string]*sidecar)
		for _, i := range groups[owner] {
			v := visitor{c: owner, program: p, file: candidates[i], opts: opts, sidecars: paths, host: host, globalPlugins: orderedHooks, customGlobals: result.CustomGlobalHooks}
			logics, plugin := v.visitFile()
			visitErrors[i] = v.err
			pluginFile[i] = plugin
			if !plugin {
				inline := v.inline(logics)
				for _, l := range logics {
					outputs[i] = append(outputs[i], l.generate(inline, metaArgument))
				}
			}
		}
	})
	for i, file := range candidates {
		if visitErrors[i] != nil {
			return nil, visitErrors[i]
		}
		if pluginFile[i] {
			result.PluginFiles = append(result.PluginFiles, file.FileName().AsString())
		}
		result.Logics = append(result.Logics, outputs[i]...)
	}
	if !result.CustomGlobalHooks && len(result.PluginFiles) == 0 {
		result.PluginContexts = nil
	}
	return result, nil
}

func first[T any](s []T) (v T) {
	if len(s) > 0 {
		return s[0]
	}
	return
}

func declaration(s *ast.Symbol) *ast.Node {
	if s == nil {
		return nil
	}
	return first(s.Declarations())
}

func walk(n *ast.Node, visit func(*ast.Node) bool) {
	if n != nil && !visit(n) {
		n.ForEachChild(func(child *ast.Node) bool { walk(child, visit); return false })
	}
}

func moduleFile(c *checker.Checker, n *ast.Node) string {
	for ; n != nil; n = n.Parent {
		if ast.IsImportDeclaration(n) {
			decl := declaration(c.GetSymbolAtLocation(n.ModuleSpecifier()))
			if decl != nil {
				return ast.GetSourceFileOfNode(decl).FileName().AsString()
			}
			return ""
		}
		if ast.IsSourceFile(n) {
			return n.AsSourceFile().FileName().AsString()
		}
	}
	return ""
}

func importedModule(c *checker.Checker, id *ast.Node) string {
	decl := declaration(c.GetSymbolAtLocation(id))
	for ; decl != nil && !ast.IsImportDeclaration(decl); decl = decl.Parent {
	}
	if decl == nil {
		return ""
	}
	return moduleFile(c, decl)
}

func collectGlobalPlugins(f *ast.SourceFile, c *checker.Checker, host *pluginHost, hooks map[string]*sidecar, ordered *[]*sidecar) bool {
	found := false
	walk(f.AsNode(), func(n *ast.Node) bool {
		if !ast.IsCallExpression(n) || !ast.IsIdentifier(n.Expression()) || n.Expression().Text() != "resetContext" {
			return false
		}
		arg := first(n.Arguments())
		if arg == nil || !ast.IsObjectLiteralExpression(arg) {
			return false
		}
		for _, p := range arg.Properties() {
			if !ast.IsPropertyAssignment(p) || !ast.IsIdentifier(p.Name()) || p.Name().Text() != "plugins" || !ast.IsArrayLiteralExpression(p.Initializer()) {
				continue
			}
			for _, id := range p.Initializer().Elements() {
				if ast.IsCallExpression(id) {
					id = id.Expression()
				}
				if file := importedModule(c, id); file != "" {
					if sc := host.load(file, true); sc != nil {
						found = true
						name := scanner.GetTextOfNode(id)
						if _, seen := hooks[name]; !seen {
							hooks[name] = sc
							*ordered = append(*ordered, sc)
						}
					}
				}
			}
		}
		return false
	})
	return found
}

func main() {
	var opts options
	err := json.NewDecoder(os.Stdin).Decode(&opts)
	if err == nil {
		var result *projectResult
		result, err = analyze(opts)
		if err == nil {
			err = json.NewEncoder(os.Stdout).Encode(result)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
