# Changelog

## v0.10.0

### Added & Enhanced

- **Always-On Zero-Trust Sandboxing**: `--sandbox-enabled=true` by default across `run`, `exec`, and `test` under the `deny-all` profile (pure computation, print, and log; isolated filesystem, network, subprocess execution, and ambient host environment).
- **Composable Capability Flags**: Modular CLI flags (`--sandbox-path-read`, `--sandbox-path-write`, `--sandbox-exec-allow`, `--sandbox-exec-deny`, `--sandbox-call-allow`, `--sandbox-call-deny`, `--sandbox-net`, `--sandbox-driver`, `--sandbox-image`, `--sandbox-enabled=false`).
- **Unified `sandbox:` Configuration Schema**: Consolidated profile definitions in `config.yaml` under `sandbox:`. High-level sandbox settings automatically synthesize fine-grained Starlark permission rules, supplemented by `calls.allow` and `calls.deny`.
- **5-Rung Capability Ladder**: Built-in profiles organized in ascending order: `deny-all` ⊂ `sandbox-path` ⊂ `sandbox-exec` ⊂ `sandbox-net` ⊂ `unconfined`.
- **Zero-Trust Environment Sanitization**: Purges ambient host environment variables by default to a safe minimal baseline (`PATH`, `HOME=/tmp`, `TMPDIR=/tmp`, `LANG=C.UTF-8`, `TERM`), paired with a 3-tier variable forwarding model (`--env KEY`, `--env KEY=VALUE`, `--env-inherit`).
- **Static AST `load()` Closure Pre-Loading**: Discovers, parses, and caches local `.star` modules referenced by top-level `load()` statements in memory prior to sandbox boundary lock down.
- **Automatic Binary & Symlink Unveiling**: Resolves permitted binaries across `$PATH`, tracing symlink chains (such as package manager links) to mount the underlying binary read-only without exposing neighboring host paths.
- **Automated Network Resolution Assets**: Enabling `--sandbox-net` or `network: host` automatically unveils system DNS resolvers (`/etc/resolv.conf`, `/etc/hosts`) and TLS CA certificate bundles.
- **Actionable Denial Diagnostics**: Sycall and Starlark permission denials include actionable `Hint:` diagnostics with exact CLI flags and `config.yaml` directives, plus debug warnings for scrubbed host environment variables.
- **Module Trust Boundary Gate**: Validates third-party module capability declarations in `mod.yaml` against the active profile before execution, halting with actionable remediation hints if requirements exceed operator grants.

### Breaking Changes

- **Legacy Permission Flags Removed**: Replaced `--permissions`, `--allow-fs`, `--allow-net`, `--allow-local`, `--allow-all`, `--sandboxed`, `--sandbox-opaque`, `--sandbox-net-access`, and `--sandbox-host` with `--sandbox-profile=<name>` and composable `--sandbox-*` flags.
- **Legacy Configuration Section Removed**: Removed the top-level `permissions:` configuration section in `config.yaml`; all capability profiles now reside under `sandbox:`.

## v0.7.5

### Breaking Changes

- **Canonical Image API Standardization**: All container image operations are now strictly standardized under the `image_*` naming convention:
  - `client.image_list(all=False)` (replaces v0.7.0 `client.images()`)
  - `client.image_pull(image, auth=None)` (replaces v0.7.0 `client.pull()`)
  - `client.image_inspect(image)`
  - `client.image_remove(image, force=False)`
  - `client.image_build(path, tag=None, dockerfile="Dockerfile")`
  All legacy aliases (`images`, `pull`, `rmi`, `build`) have been removed from both `Client` and the `containers` module shortcuts.

### Added & Enhanced

- **Containers Module Shortcuts**: Top-level shortcuts on the `containers` module for direct single-line execution (`containers.image_pull`, `containers.image_build`, `containers.image_list`, `containers.image_inspect`, `containers.image_remove`, `containers.run`, `containers.exec`, `containers.stop`, `containers.delete`).
- **Apex Domain Install Distribution**: Edge routing via Cloudflare Worker for `https://starkite.run/install.sh` and `https://starkite.run/install.ps1`.
- **Responsive Documentation UI**: CSS container queries (`hero-terminal`) and fluid typography (`clamp()`) for code terminals and install commands; unified elevated code card styling across docs.

### Fixed

- **Windows Named Pipe Hanging**: Implemented active deadline enforcement (`SetReadDeadline`, `SetWriteDeadline`) on Windows named pipes (`//./pipe/docker_engine`) using `time.Timer` and `windows.CancelIoEx`, resolving blocking I/O on broken pipes or daemon aborts.

## Unreleased

### Breaking changes — `--sandbox` and `--trust` removed

- The `--sandbox` flag is replaced by `--permissions=<profile>`. Use `--permissions=strict` for the previous `--sandbox` behavior. No alias is kept (pre-release).
- The `--trust` flag is removed. Trust mode is the default when `--permissions` is unset; an explicit profile name (`--permissions=trusted`) lands in a follow-up phase.

### Breaking changes — naming refactor

The repository has been restructured so that every directory and binary name conveys its intent at a glance. Pre-release, no migration aliases.

**Directory layout** — every module directory ends in `kite`:

- `libkite/` — embeddable Starlark runtime. Import path `github.com/project-starkite/starkite/libkite`; package `libkite`.
- `kite/` — composes every edition into one binary; produces `kite`.
- `basekite/` — lean base CLI; produces `kitecmd`.
- `cloudkite/` — base + Kubernetes; produces `kitecloud`.
- `aikite/` — base + LLM/MCP; produces `kiteai`.

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
- **Edition-namespace disjointness invariant** — enforced by the all-edition's loader test (`kite/loader/loader_test.go`). Any future PR that registers a colliding name across editions fails CI.

## v0.0.1 — Initial Release

The first public release of **starkite** (formerly starctl/crsh), an automation language that exposes Go's standard library as type-safe, scriptable Starlark modules. Built on [Starlark](https://github.com/google/starlark-go).

### Highlights

- **25+ built-in modules** — os, fs, http (client + server), ssh, json, yaml, csv, gzip, zip, base64, hash, strings, regexp, template, time, uuid, log, fmt, table, concur, retry, io, vars, runtime, inventory, test
- **Two editions** — Base (default) and Cloud (Kubernetes, Helm, container modules)
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
