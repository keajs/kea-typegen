package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPortablePlugin(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sdk, err := filepath.Abs("../plugin")
	if err != nil {
		t.Fatal(err)
	}
	write("go.mod", "module fixture\n\ngo 1.23\nrequire github.com/keajs/kea-typegen/plugin v1.0.0\nreplace github.com/keajs/kea-typegen/plugin => "+strconv.Quote(filepath.ToSlash(sdk))+"\n")
	write("main.go", `package main
import "github.com/keajs/kea-typegen/plugin"
func main(){plugin.Serve(func(r plugin.Request)(plugin.Result,error){
	if !r.SupportsLogicFieldMembers { return plugin.Result{Delegate:true},nil }; return plugin.Result{LogicFieldMembers:map[string][]plugin.Value{"workers": {{Name:r.Properties[0].Name,Type:"Saga"}}}, Reducers: []plugin.Value{{Name:r.Properties[0].Name, Type:r.Properties[0].Type}}},nil
})}`)
	write("node_modules/widget/package.json", `{"name":"widget","types":"index.d.ts"}`)
	write("node_modules/widget/index.d.ts", "export declare function widget(x:unknown):unknown")
	write("node_modules/widget/typegen.json", `{"version":1,"module":"builder.wasm","builders":["widget"],"globals":["widget"],"typePaths":[["*"]]}`)
	write("node_modules/widget/typegen.js", "exports.widget = () => {}")
	cmd := exec.Command("go", "build", "-o", "node_modules/widget/builder.wasm", ".")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0", "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %s: %v", output, err)
	}
	write("tsconfig.json", `{"compilerOptions":{"module":"nodenext"},"files":["logic.ts"]}`)
	write("logic.ts", `import {widget} from "widget"
declare function kea(x:unknown):unknown
declare function resetContext(x:unknown):void
resetContext({plugins:[widget]})
export const first = kea([widget({count:0}), widget({more:1})])
export const second = kea({widget:{text:"hello"}})`)
	opts := options{RootPath: root, TsConfigPath: filepath.Join(root, "tsconfig.json"), Inline: true}
	result, err := analyze(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Logics) != 2 || len(result.PluginFiles) != 0 || len(result.PluginContexts) != 0 ||
		!strings.Contains(result.Logics[0].Declaration, "count: number") || !strings.Contains(result.Logics[1].Declaration, "text: string") {
		t.Fatal("portable builder or global dispatch failed:", result.Logics)
	}
	for _, inline := range []bool{true, false} {
		opts.Inline = inline
		result, err := analyze(opts)
		if err != nil {
			t.Fatal(err)
		}
		decl := result.Logics[0].Declaration
		if strings.Count(decl, "workers:") != 1 || !strings.Contains(decl, "count: Saga;") || !strings.Contains(decl, "more: Saga;") {
			t.Fatalf("missing merged logic fields, inline=%v: %s", inline, decl)
		}
	}
	t.Setenv("KEA_TYPEGEN_JS_PLUGINS", "1")
	result, err = analyze(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PluginFiles) != 1 || !result.CustomGlobalHooks {
		t.Fatal("expected JS compatibility for the same project")
	}
}
