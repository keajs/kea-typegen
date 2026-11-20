# Go plugin API v1

A plugin can ship a Go visitor with its npm package. Typegen runs the compiled WebAssembly command with WASI, once per matching builder or global property, inside the Go worker. The command receives JSON on stdin and writes its result to stdout. The same module works on all host platforms. The SDK requires only the Go standard library.

Import `github.com/keajs/kea-typegen/plugin` in a Go command and call `plugin.Serve`. For example, this builder adds one reducer per property of its input object.

```go
package main

import "github.com/keajs/kea-typegen/plugin"

func main() {
    plugin.Serve(func(input plugin.Request) (plugin.Result, error) {
        var out plugin.Result
        for _, field := range input.Properties {
            out.Reducers = append(out.Reducers, plugin.Value{Name: field.Name, Type: field.Type})
        }
        return out, nil
    })
}
```

Place a `typegen.json` next to the resolved TypeScript declarations of the package that exports the builder.

```json
{
  "version": 1,
  "module": "typegen.wasm",
  "builders": ["fields"],
  "globals": ["fields"],
  "typePaths": [["*"]]
}
```

Build the command with `GOOS=wasip1 GOARCH=wasm go build -trimpath -o lib/typegen.wasm .`, copy the manifest into `lib`, and include both in the npm package. The SDK source also ships under `kea-typegen/plugin` for local development with a Go module replacement.

- `builders` declares the names handled on imported builder calls. `globals` declares the names handled when the plugin is registered via `resetContext({ plugins: [...] })`. The resolved builder's visitor takes precedence on a direct match. Global hooks run in registration order.
- Each request includes the file, logic name, builder name, argument source text, syntax kind and named object members. Typegen also recognizes an arrow function returning a parenthesized object literal. A member exposes its source name, member kind, name kind and initializer kind.
- `typePaths` selects the member types the Go checker should infer. Each segment matches a property name or `*`. For example, `[["*", "defaults"]]` makes the type of each form's defaults available through its nested properties. This avoids asking the checker to infer the types of unrelated callbacks.
- Return actions, reducers, selectors and imports in declaration order. Action parameters carry their names, TypeScript types and optional flags. Return an error for a failed invocation. The host reports the manifest and logic name.
- Keep a JavaScript visitor alongside the module for older typegen versions, classic/native mode, and files that mix in arbitrary JavaScript visitors. A portable visitor can also return `Delegate: true` for input it wants that visitor to handle. Use `KEA_TYPEGEN_JS_PLUGINS=1` with `--compiler go` to compare both implementations when a JS companion is available.

The request and result types are defined in [plugin.go](plugin.go).
