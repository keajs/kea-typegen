package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
)

func TestGenerateProject(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("tsconfig.json", `{"compilerOptions":{"strict":true},"files":["logic.ts"]}`)
	write("models.ts", `export interface Row { id: number }; export declare function lookup(): Row`)
	write("logic.ts", `// 🦜
import { lookup, type Row } from './models'
declare function kea<T>(o: unknown): T
type manualType = { custom: true }
export const manual = kea<manualType>({})
export const docs = kea({
    path: ['docs'],
    props: {} as { owner: Row },
    actions: { lookup: () => lookup(), 'set-title': (title: string) => ({ title }) },
    loaders: { rows: [[] as Row[], {
        load: async (args: { id: Row['id'] }) => [lookup()]
    }] },
    selectors: { total: [() => [], (): number => 1] }
})`)
	for _, inline := range []bool{true, false} {
		got, err := analyze(options{TsConfigPath: filepath.Join(root, "tsconfig.json"), RootPath: root, Inline: inline})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Logics) != 1 || len(got.PluginFiles) != 0 {
			t.Fatalf("got %d managed logics and %d plugin files", len(got.Logics), len(got.PluginFiles))
		}
		l := got.Logics[0]
		if l.Name != "docs" || !l.HasPath {
			t.Fatalf("unexpected logic %q with path %v", l.Name, l.Path)
		}
		for _, text := range []string{"Row", "loadSuccess", "rowsLoading", "loadFailure", "set-title", "total", "number", "id"} {
			if !strings.Contains(l.Declaration, text) {
				t.Errorf("missing %s in %s", text, l.Declaration)
			}
		}
		if !strings.Contains(l.Declaration, `Row["id"]`) && !strings.Contains(l.Declaration, `Row['id']`) {
			t.Errorf("lost indexed access key: %s", l.Declaration)
		}
		if strings.Contains(l.Declaration, "MakeLogicType") != inline {
			t.Errorf("unexpected output mode: %s", l.Declaration)
		}
		f := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/declaration.ts", PathKey: "/declaration.ts"}, l.Declaration, core.ScriptKindTS)
		if len(f.Diagnostics()) != 0 {
			t.Fatalf("invalid generated declaration: %s", l.Declaration)
		}
	}
}
