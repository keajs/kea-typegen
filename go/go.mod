// The pinned compiler exposes its Go API under internal/. Keep the integration
// in its import namespace so Go enforces that boundary everywhere else.
module github.com/microsoft/TypeScript/tsc/kea-typegen

go 1.27

require (
	github.com/keajs/kea-typegen/plugin v1.1.0
	github.com/microsoft/TypeScript/tsc v0.0.0-20261019203854-3e9236361fa9
	github.com/tetratelabs/wazero v1.12.0
	golang.org/x/text v0.42.0
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/keajs/kea-typegen/plugin => ../plugin
