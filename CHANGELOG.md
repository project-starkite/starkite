# Changelog

## Unreleased

### Breaking changes

- **Edition rename to `<edition>kite` form.** Source directories renamed: `core/` → `basekite/`, `cloud/` → `cloudkite/`, `ai/` → `aikite/`, plus a new `allkite/` directory for the all-in-one. Go module paths follow the same shape (`github.com/project-starkite/starkite/<edition>kite`).
- **Binary name changes:** the lean base binary `kite` is now `basekite`; `kite-cloud` is now `cloudkite`; `kite-ai` is now `aikite`. The unadorned `kite` binary now refers to the new **all-in-one edition** (base + Kubernetes + GenAI/MCP). Install `basekite` if you want the lean base experience.
- **Local builds land in `./bin/`** rather than at the project root, since each edition's source directory now shares its name with the produced binary.

### Added

- **`kite` (all-in-one edition)** — bundles every module from every edition in one binary. ~92 MB vs. ~26 MB for `basekite`. Use it when you want a single binary that covers every workflow.
- **Strict-mode registry** in `starbase` — `Registry.SetStrict(true)` causes module-name, top-level export, and global-alias collisions to surface at startup instead of silently overwriting. The all-edition opts in; the lean editions stay lenient.
- **Edition-namespace disjointness invariant** — enforced by the all-edition's loader test (`allkite/loader/loader_test.go`). Any future PR that registers a colliding name across editions fails CI.

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
