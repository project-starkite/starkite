module github.com/vladimirvivien/starkite/wasm

go 1.26

require (
	github.com/extism/go-sdk v1.7.1
	github.com/vladimirvivien/starkite/starbase v0.0.0
	github.com/vladimirvivien/startype v0.7.1
	go.starlark.net v0.0.0-20260326113308-fadfc96def35
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/dylibso/observe-sdk/go v0.0.0-20240819160327-2d926c5d788a // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/ianlancetaylor/demangle v0.0.0-20240805132620-81f5be970eca // indirect
	github.com/tetratelabs/wabin v0.0.0-20230304001439-f6f874872834 // indirect
	github.com/tetratelabs/wazero v1.9.0 // indirect
	go.opentelemetry.io/proto/otlp v1.3.1 // indirect
	golang.org/x/sys v0.42.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/vladimirvivien/starkite/starbase => ../starbase
