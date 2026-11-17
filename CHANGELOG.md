# Changelog

## v0.11.9

### Added & Enhanced

- **Kubernetes Object Constructors (`k8s.obj`)**:
  - Added `k8s.obj.role_ref(name, kind="Role", api_group="rbac.authorization.k8s.io")` for typed RBAC bindings (`RoleBinding` and `ClusterRoleBinding`), plus direct `k8s.obj.role(...)` and `k8s.obj.cluster_role(...)` object conversion and full backward compatibility with raw dict `role_ref` inputs.
  - Added `k8s.obj.priority_class(name, value, ...)` (`scheduling.k8s.io/v1`, alias `k8s.obj.pc`) and `priority_class_name` support on `k8s.obj.pod`, `k8s.obj.pod_spec`, `k8s.obj.pod_template`, and workload constructors (`deployment`, `stateful_set`, `daemon_set`, `job`, `cron_job`).
  - Added `k8s.obj.validating_webhook_config(name, webhooks=[], ...)` and `k8s.obj.mutating_webhook_config(name, webhooks=[], ...)` (`admissionregistration.k8s.io/v1`).
  - Generalized positional YAML/dict extraction across top-level resource fields and nested `KubeResource` resolution within `FieldKubeObject` dicts.
- **Built-In Kubernetes GVR Shortcuts**: Added fast O(1) resolution entries for `resourcequota`/`quota`, `limitrange`/`limits`, `priorityclass`/`pc`, `validatingwebhookconfiguration`, `mutatingwebhookconfiguration`, `validatingadmissionpolicy`/`vap`, `validatingadmissionpolicybinding`, and `customresourcedefinition`/`crd`.
- **API Reference & Agent Skills Sync**: Documented all newly added and existing RBAC, scheduling, admission, network, and pod sub-object `k8s.obj` constructors in `docs/references/api/k8s.md` and synchronized `libkite/catalog/apidocs/table_gen.go` and `.agent/skills/`.

### Removed

- **Unimplemented `kite fmt` Help Reference**: Removed the unimplemented `kite fmt ./scripts/` example from the root CLI help text.

### Fixed

- **Linux Landlock Executable Allowlist Hardening**: Restricted Landlock library execute rules to ELF dynamic linkers (`PT_INTERP`) rather than broad `/usr/lib` and `/usr/libexec` trees, and enforced invoked-path allowlist verification for multicall binaries (such as `uutils` `coreutils` and `busybox`).
- **Pre-Flight Race Test Exit Propagation**: Enforced fail-fast exit status propagation across multi-module `go test -race` runs in `Makefile`.

## v0.11.8

### Added & Enhanced

- **Editor & Agent Skills Examples**: Added multi-editor LSP configurations (`examples/lsp/`), agent skills documentation (`docs/ai/skills.md`), operational repair scripts (`examples/skills/`), landing page LSP intelligence section, and VS Code extension icon/packaging polish.

## v0.11.7

### Added & Enhanced

- **`kite doc` Reference & Lint Guidance**: Added CLI reference documentation (`docs/references/cli/doc.md`), Starlark native string method lint rules (`libkite/lint`), and corresponding agent skill guidance.

### Fixed

- **Sandbox Config Tolerance**: Tolerate permission denied errors when probing optional default user configuration files under strict filesystem confinement.

## v0.11.4 – v0.11.6

### Added & Enhanced

- **Embedded `kite lsp` & Static Linter**: Consolidated `kite lsp` (`--stdio`) directly into the unified `kite` binary, expanded dialect and sandbox lint checks in `kite validate --lint`, added closed-loop agent dogfooding tests (`tests/dogfood`), and packaged the VS Code extension `.vsix` in release builds.

## v0.11.0 – v0.11.3

### Added & Enhanced

- **Language Server, Reference Catalog & Agent Skills**: Implemented Starlark LSP diagnostics/completions/hover (`libkite/lsp`), extracted `libkite/catalog` with `TypedModule` metadata and automated `gendocs` synchronization, introduced `kite doc` (`--json` and `--skills`), and automated `project-starkite/skills` synchronization on release.

## v0.10.10

### Changed

- **Documentation Modernization**: Updated the root README, landing page security showcase, and sandbox guide around the execution triad, explicit `--sandbox-net-allow`/`--sandbox-net-deny`, and `--sandbox-mount`.

## v0.10.9

### Added & Enhanced

- **First-Class `--sandbox-mount` CLI Flag**: Repeatable `--sandbox-mount` flag for explicit host-to-sandbox path bind mounts (`<source>[:<dest>[:ro|rw]]`), identity mounts (`<source>[:<mode>]`), and ephemeral in-memory filesystems (`tmpfs:<dest>[:ro|rw]`). Mounts are enforced across Linux (Landlock), macOS (Seatbelt), Windows (AppContainers), and OCI container runtimes.
- **Repeatable-Only Path Overrides**: `--sandbox-path-read` and `--sandbox-path-write` flags are strictly repeatable without comma splitting, preserving literal commas in filesystem paths without parsing ambiguity.
- **Driver Validation Support for Path Commas**: Mount source and destination path validation allows literal commas while retaining strict rejection of colons outside volume specifiers, quotes, newlines, and NUL characters.
- **Actionable Interpreter Diagnostics**: Missing dependency diagnostics across Python (`ModuleNotFoundError`), Node.js (`cannot find module`), and Ruby (`LoadError`) suggest explicit `--sandbox-path-read` or `--sandbox-mount` grants.

### Removed

- **Hard-Coded Language Runtime Profiles**: Removed built-in language environment profiles (`python-env`, `node-env`, `ruby-env`) and automated host filesystem scanning heuristics in favor of explicit, composable mount and path primitives.

## v0.10.6 – v0.10.8

### Added & Enhanced

- **Execution Triad (`kite run` / `kite exec` / `kite eval`)**: Added native process confinement via `kite exec`, migrated inline Starlark evaluation to `kite eval`, enforced non-Starlark guardrails in `kite run`, and enriched interpreter denial diagnostics.
- **Explicit Network Controls**: Replaced `--sandbox-net` with symmetric `--sandbox-net-allow` / `--sandbox-net-deny` flags, `--sandbox-net-mode`, `--sandbox-max-memory`, and endpoint-scoped enforcement across Linux, macOS, and Windows drivers.

## v0.10.1 – v0.10.5

### Added & Enhanced

- **Native Windows Kernel Sandbox**: Delivered native Windows 11 24H2+ / Windows Server 2025+ isolation (`--sandbox-driver=windows` / `appcontainer`) using Win32 Job Objects, `STARTUPINFOEXW` child-process mitigations, ephemeral AppContainer profiles with WFP network gating, surgical NTFS DACL unveiling with `W^X` scratch locks, lease garbage collection, Win32 denial diagnostics, and default sandbox activation.

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

## v0.0.1 — Initial Release

The first public release of **starkite**, an automation language that exposes Go's standard library as type-safe, scriptable Starlark modules. Built on [Starlark](https://github.com/google/starlark-go).

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
