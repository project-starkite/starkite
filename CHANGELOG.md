# Changelog

## v0.0.1 — Initial Release

The first public release of **starkite** (formerly starctl/crsh), an automation language that exposes Go's standard library as type-safe, scriptable Starlark modules. Built on [Starlark](https://github.com/google/starlark-go).

### Highlights

- **25+ built-in modules** — os, fs, http (client + server), ssh, json, yaml, csv, gzip, zip, base64, hash, strings, regexp, template, time, uuid, log, fmt, table, concur, retry, io, vars, runtime, inventory, test
- **Two editions** — Base (default) and Cloud (Kubernetes, Helm, container modules)
- **WASM plugin system** — extend starkite with WebAssembly modules via `module.yaml` manifests
- **Permission system** — `--trust` (default, allow all) and `--sandbox` (restrict to safe operations)
- **Built-in test runner** — `kite test` with parallel execution, filtering, setup/teardown, skip support
- **Interactive REPL** — `kite repl` for exploratory scripting
- **Variable injection** — 5-tier priority resolution (CLI > files > config > env > script defaults)
- **File watcher** — `kite watch` for auto-re-execution on file changes
- **Module system** — `load()` supports single-file, multi-file (directory), and external modules
- **Edition handoff** — base binary auto-delegates to cloud binary when activated
- **27 example scripts** — core examples + 12 Kubernetes deployment patterns

### CLI Binary

The CLI binary is named `kite`:

```
kite run script.star
kite test ./tests/
kite exec 'print("hello")'
kite repl
kite watch script.star
kite init --template=kubernetes
kite version
```

### Editions

| Edition | Binary | Modules |
|---------|--------|---------|
| Base | `kite` | 25+ core modules |
| Cloud | `kite-cloud` | Core + k8s, helm, container, cloud |

### Platforms

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)
