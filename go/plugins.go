package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	api "github.com/keajs/kea-typegen/plugin"
	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type manifest struct {
	Version           int
	Module            string
	Builders, Globals []string
	TypePaths         [][]string
}

type sidecar struct {
	js bool
	*manifest
	module wazero.CompiledModule
	path   string
	err    error
}

type pluginHost struct {
	mu      sync.Mutex
	runtime wazero.Runtime
	files   map[string]*sidecar
}

func (h *pluginHost) close() {
	if h.runtime != nil {
		h.runtime.Close(context.Background())
	}
}

// Resolve once per project. Parallel checkers share the compiled module, and
// each invocation gets its own WASI instance and streams.
func (h *pluginHost) load(file string, global bool) *sidecar {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := file
	if global {
		key += ":global"
	}
	if sc, ok := h.files[key]; ok {
		return sc
	}
	sc := &sidecar{}
	h.files[key] = sc
	dir := filepath.Dir(file)
	stem := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)), ".d")
	names := []string{"typegen"}
	if !global {
		names = append(names, stem+".typegen")
	}
	for _, name := range names {
		base := filepath.Join(dir, name)
		for _, ext := range []string{".js", ".ts"} {
			if _, err := os.Stat(base + ext); err == nil {
				sc.js = true
				break
			}
		}
		data, err := os.ReadFile(base + ".json")
		if err == nil {
			m := new(manifest)
			sc.path = base + ".json"
			if existing := h.files[sc.path]; existing != nil {
				h.files[key] = existing
				return existing
			}
			h.files[sc.path] = sc
			if err := json.Unmarshal(data, m); err != nil {
				sc.err = err
				return sc
			}
			if m.Version != api.Version || (sc.js && os.Getenv("KEA_TYPEGEN_JS_PLUGINS") == "1") {
				if !sc.js {
					sc.err = fmt.Errorf("unsupported plugin version %d", m.Version)
				}
				return sc
			}
			if !filepath.IsLocal(m.Module) {
				sc.err = fmt.Errorf("invalid module path %q", m.Module)
				return sc
			}
			sc.manifest = m
			ctx := context.Background()
			if h.runtime == nil {
				h.runtime = wazero.NewRuntime(ctx)
				if _, err := wasi_snapshot_preview1.Instantiate(ctx, h.runtime); err != nil {
					sc.err = err
					return sc
				}
			}
			wasm, err := os.ReadFile(filepath.Join(dir, m.Module))
			if err == nil {
				sc.module, err = h.runtime.CompileModule(ctx, wasm)
			}
			sc.err = err
			return sc
		}
		if !os.IsNotExist(err) {
			sc.err = err
			sc.path = base + ".json"
			return sc
		}
		if sc.js {
			return sc
		}
	}
	h.files[key] = nil
	return nil
}

func (v *visitor) runPlugin(sc *sidecar, name string, arg *ast.Node) (bool, error) {
	request := api.Request{Version: api.Version, Builder: name, File: v.l.File, Logic: v.l.Name, Argument: scanner.GetTextOfNode(arg), Kind: strings.TrimPrefix(arg.Kind.String(), "Kind")}
	node := arg
	if ast.IsArrowFunction(node) && ast.IsParenthesizedExpression(node.Body()) {
		node = node.Body().Expression()
	}
	request.Properties = v.pluginProperties(node, sc.TypePaths)
	in, err := json.Marshal(request)
	if err != nil {
		return false, err
	}
	var out, diagnostics bytes.Buffer
	config := wazero.NewModuleConfig().WithName("").WithStdin(bytes.NewReader(in)).WithStdout(&out).WithStderr(&diagnostics)
	ctx := context.Background()
	instance, err := v.host.runtime.InstantiateModule(ctx, sc.module, config)
	if err != nil {
		return false, fmt.Errorf("%s in %s: %w %s", sc.path, v.l.Name, err, diagnostics.String())
	}
	defer instance.Close(ctx)
	var result api.Result
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return false, fmt.Errorf("%s: %w", sc.path, err)
	}
	if result.Delegate {
		if !sc.js {
			return false, fmt.Errorf("%s requested JavaScript delegation without a JavaScript visitor", sc.path)
		}
		return false, nil
	}
	for _, a := range result.Actions {
		params := make([]parameter, len(a.Params))
		for i, p := range a.Params {
			params[i] = parameter(p)
		}
		v.l.Actions = append(v.l.Actions, action{Name: a.Name, Type: a.Type, Params: params})
	}
	for _, r := range result.Reducers {
		v.l.Reducers = append(v.l.Reducers, value{Name: r.Name, Type: r.Type})
	}
	for _, s := range result.Selectors {
		v.l.Selectors = append(v.l.Selectors, value{Name: s.Name, Type: s.Type})
	}
	for _, i := range result.Imports {
		v.addImport(i.File, i.Name)
	}
	return true, nil
}

func (v *visitor) pluginProperties(node *ast.Node, paths [][]string) []api.Property {
	if node == nil || !ast.IsObjectLiteralExpression(node) {
		return nil
	}
	var props []api.Property
	for _, p := range node.Properties() {
		if p.Name() == nil {
			continue
		}
		key := scanner.GetTextOfNode(p.Name())
		item := api.Property{Name: key, Kind: strings.TrimPrefix(p.Kind.String(), "Kind"), NameKind: strings.TrimPrefix(p.Name().Kind.String(), "Kind")}
		var nested [][]string
		for _, path := range paths {
			if len(path) == 0 || (path[0] != "*" && path[0] != key) {
				continue
			}
			if len(path) == 1 {
				item.Type = v.inferred(v.c.GetTypeAtLocation(p))
			} else {
				nested = append(nested, path[1:])
			}
		}
		if ast.IsPropertyAssignment(p) {
			item.ValueKind = strings.TrimPrefix(p.Initializer().Kind.String(), "Kind")
			if len(nested) > 0 {
				item.Properties = v.pluginProperties(p.Initializer(), nested)
			}
		}
		props = append(props, item)
	}
	return props
}

func (v *visitor) pluginsFor(name string, direct *sidecar, arg *ast.Node, delegated bool) bool {
	run := func(sc *sidecar) bool {
		if !sc.js {
			v.goOnlyPlugin = sc.path
		}
		if delegated || v.err != nil {
			return false
		}
		ok, err := v.runPlugin(sc, name, arg)
		v.err = err
		return ok
	}
	if direct != nil {
		if direct.err != nil {
			v.err = fmt.Errorf("%s: %w", direct.path, direct.err)
			return false
		}
		if direct.manifest != nil && slices.Contains(direct.Builders, name) {
			if !v.customGlobals {
				return run(direct)
			}
			if !direct.js {
				v.err = fmt.Errorf("%s needs a JavaScript visitor alongside custom global JavaScript hooks", direct.path)
			}
			return false
		}
		if direct.js {
			return false
		}
	}
	if !v.customGlobals {
		for _, sc := range v.globalPlugins {
			if slices.Contains(sc.Globals, name) && !run(sc) {
				delegated = true
			}
		}
		return !delegated
	}
	return true
}
