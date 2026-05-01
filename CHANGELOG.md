# Changelog

## Unreleased

### Breaking changes — `--sandbox` and `--trust` removed

- The `--sandbox` flag is replaced by `--permissions=<profile>`. Use `--permissions=strict` for the previous `--sandbox` behavior. No alias is kept (pre-release).
- The `--trust` flag is removed. Trust mode is the default when `--permissions` is unset; an explicit profile name (`--permissions=trusted`) lands in a follow-up phase.

### Breaking changes — naming refactor

The repository has been restructured so that every directory and binary name conveys its intent at a glance. Pre-release, no migration aliases.

**Directory layout** — infrastructure packages use a `<descriptor>kite/` form; domain editions are bare nouns:

- `libkite/` — embeddable Starlark runtime (was `starbase/`, briefly `kitecore/`). Import path `github.com/project-starkite/starkite/libkite`; package `libkite`.
- `allkite/` — composes every edition into one binary; produces `kite`.
- `base/` — lean base CLI (was `core/`, briefly `basekite/`); produces `kitecmd`.
- `cloud/` — base + Kubernetes (was `cloud/`, briefly `cloudkite/`); produces `kitecloud`.
- `ai/` — base + LLM/MCP (was `ai/`, briefly `aikite/`); produces `kiteai`.

**Binaries** use the `kite<edition>` prefix form (with the all-in-one as plain `kite`):

| Binary | Modules | Replaces |
|---|---|---|
| `kite` | base + Kubernetes + GenAI/MCP (all-in-one) | (new) |
| `kitecmd` | base only | `kite` (lean base) |
| `kitecloud` | base + Kubernetes | `kite-cloud` |
| `kiteai` | base + LLM/MCP | `kite-ai` |

**Type references** switch from `starbase.Registry`/`starbase.Module`/`starbase.Config` to `libkite.Registry`/`libkite.Module`/`libkite.Config`.

**Local builds land in `./bin/`** rather than at the project root, since each edition's source directory could otherwise collide with the produced binary's name in earlier iterations.

### Added

- **`kite` (all-in-one edition)** — bundles every module from every edition in one binary. ~92 MB vs. ~26 MB for `kitecmd`. Recommended for new users.
- **Strict-mode registry** in `libkite` — `Registry.SetStrict(true)` causes module-name, top-level export, and global-alias collisions to surface at startup instead of silently overwriting. The all-edition opts in; the lean editions stay lenient.
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
