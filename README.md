<p align="center">
    <img src="./docs/assets/images/starkite-logo-banner-bg.png" alt="Starkite" width="550">
    <h3 align="center"> Secure Runtime for Cloud-Native and Agentic AI Automation with Starlark </h3> 
</p>

**starkite** is an automation language built on [Starlark](https://github.com/google/starlark-go) (a Python-like language). It exposes Go's standard library as type-safe, scriptable Starlark modules — providing a unified interface for general-purpose, cloud-native, and MCP agent automation.

## Features

- **Execution Triad** — Execute Starlark scripts (`kite run`), run native host binaries under kernel isolation (`kite exec`), or evaluate inline expressions (`kite eval`)
- **Zero-trust sandbox by default** — Kernel-enforced isolation (Linux Landlock ABI v1–v5 + Seccomp-BPF, macOS Seatbelt, Windows AppContainers + Job Objects) paired with in-VM Starlark call gates
- **First-class filesystem mounts** — Composable host bind mounts (`--sandbox-mount src:dst:ro`), identity mounts, and isolated memory scratch filesystems (`tmpfs:dest`)
- **Go stdlib as modules** — Type-safe, scriptable access to Go's standard library
- **Python-like syntax** — Uses Starlark, a deterministic, hermetic Python dialect
- **General-purpose automation** — System tasks, scripting, data processing
- **Cloud-native operations** — Kubernetes integration, infrastructure management (Cloud edition)
- **Agent automation** — Tool execution and MCP integration for AI agents
- **27+ built-in modules** — OS, filesystem, HTTP (client + server), SSH, JSON, YAML, CSV, concurrency, retry, and more
- **SSH operations** — Multi-host concurrent execution, jump hosts, SCP upload/download
- **Resilience patterns** — Retry with exponential backoff, concurrent map/each/exec
- **Built-in test runner** — Test framework with assertions, filtering, and setup/teardown

## Editions

The default binary is **`kite`**, the all-in-one edition — it bundles every module and is what all examples and documentation use. The lean editions are smaller subsets of the same binary, for space-conscious targets.

| Binary | Adds on top of base | Use when |
|---|---|---|
| `kite` | base + Kubernetes + MCP (all-in-one) | you want everything in one binary |
| `kitecmd` | base modules only (os, fs, http, ssh, json, yaml, time, log, …) | system scripts, CI tasks, general automation |
| `kitecloud` | base + Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kiteai` | base + MCP server and client | MCP tools and agent orchestration |

Use `kite` unless binary size or attack surface is a real constraint — init containers, edge nodes, or CI runners under a restricted profile such as `--sandbox-profile=sandbox-path`. The lean editions (`kitecmd` / `kitecloud` / `kiteai`) are a strict subset for those targets.

> **Naming convention.** Source directory names end in `kite`: `libkite/` (embeddable runtime), `basekite/` / `cloudkite/` / `aikite/` (editions), and `kite/` (the all-in-one). Binaries use the `kite<edition>` prefix form (`kitecmd`, `kitecloud`, `kiteai`), with the all-in-one as the unadorned `kite`.

## Installation

Install the all-in-one `kite` binary using your preferred method:

### Package Managers

* **macOS / Linux (Homebrew)**:
  ```bash
  brew install project-starkite/tap/kite
  ```

* **Linux / macOS (POSIX Installer Script)**:
  ```bash
  curl -fsSL https://starkite.run/install.sh | sh
  ```

* **Windows (PowerShell Installer Script)**:
  ```powershell
  irm https://starkite.run/install.ps1 | iex
  ```

* **Windows (Scoop)**:
  ```powershell
  scoop bucket add starkite https://github.com/project-starkite/scoop-bucket
  scoop install kite
  ```

### Direct Download

Pre-built binaries for Linux, macOS, and Windows are available on [GitHub Releases](https://github.com/project-starkite/starkite/releases) (`kite-linux-amd64`, `kite-darwin-arm64`, `kite-windows-amd64.exe`, etc.).

### Container Image

Run `kite` directly via the signed Chainguard-based OCI image:

```bash
docker run --rm ghcr.io/project-starkite/starkite:latest eval 'print(runtime.platform())'
```

### Build from Source

The repository is configured as a Go workspace:

```bash
git clone https://github.com/project-starkite/starkite.git
cd starkite

make kite               # ./bin/kite — the default all-in-one
# lean editions (optional, smaller footprint):
make build-base         # ./bin/kitecmd    (base only)
make build-cloud        # ./bin/kitecloud  (base + k8s)
make build-ai           # ./bin/kiteai     (base + MCP)
make all                # all four at once
```

See the [Installation Guide](docs/getting-started/install.md) for full configuration, signature verification, and container workflows.

## Quick Start

```python
#!/usr/bin/env kite
# hello.star

name = var_str("name", "World")
printf("Hello, %s!\n", name)
printf("Running on %s/%s\n", runtime.platform(), runtime.arch())

# Execute a command
result = os.exec("uname -a")
print(result)
```

Then, make the script executable and run the script:

```bash
chmod +x hello.star
./hello.star --sandbox-exec-allow=uname --var name=Alice
```

Or, run it directly with `kite`:

```bash
kite hello.star --sandbox-exec-allow=uname
kite hello.star --sandbox-exec-allow=uname --var name=Alice
```

## CLI

The Starkite CLI is structured around the **execution triad** (`run`, `exec`, `eval`) alongside development and verification utilities:

```bash
# Execution Triad
kite run script.star                                # Execute Starlark script under kernel sandbox
kite exec --sandbox-mount ./data:/data:ro -- curl   # Run native process in kernel sandbox (no Starlark VM)
kite eval 'print(1 + 1)'                            # Evaluate inline Starlark expression

# Development & Operations
kite repl                                           # Interactive REPL
kite test ./tests/                                  # Run test suites
kite watch script.star                              # Re-run script on file changes
kite validate script.star                           # Verify script syntax and static structure
kite module install <repo>                          # Install external Starlark module
kite update                                         # Self-update binary
```

See the [CLI reference](https://starkite.dev/references/cli/) for all commands and flags.

## Modules

All modules are auto-loaded — no import statements needed:

```python
# Filesystem
content = fs.read_text("config.yaml")
fs.path("/tmp/output").write_text("hello")

# HTTP client
resp = http.url("https://api.example.com/data").get()
data = json.decode(resp.body)

# SSH (multi-host concurrent execution)
client = ssh.config(hosts=["web1", "web2", "web3"], auth={"user": "admin"})
results = client.exec("uptime")

# Concurrency
results = concur.map(hosts, check_host, workers=4, timeout="30s")

# Retry with backoff
result = retry.with_backoff(flaky_op, max_attempts=5)

# Data processing
records = csv.file("data.csv").read(header=True)
yaml.source(data).write_file("output.yaml")
```

| Category | Modules |
|----------|---------|
| System | `os`, `fs`, `io`, `runtime` |
| Data | `json`, `yaml`, `csv`, `template`, `base64`, `hash`, `gzip`, `zip` |
| Text | `strings`, `regexp`, `fmt` |
| Network | `http` (client + server), `ssh`, `inventory` |
| Execution | `concur`, `retry` |
| Utility | `time`, `uuid`, `log`, `table`, `vars`, `path` |
| Testing | `test` (assert, assert_equal, assert_contains, skip) |
| Cloud | `k8s` (in `kite` and `kitecloud`) |
| AI | `mcp` (in `kite` and `kiteai`) |

See the [API reference](https://starkite.dev/references/api/) for full module documentation.

## Zero-Trust Sandboxing & Capability Profiles

Starkite enforces zero-trust isolation by default. Every script (`kite run`) and isolated command (`kite exec`) runs inside an OS-level kernel sandbox (Linux Landlock ABI v1–v5 + Seccomp-BPF, macOS Seatbelt, Windows AppContainers + Job Objects) under the `deny-all` profile: pure computation, print, and log are allowed, while host filesystem, outbound network, subprocess execution, and ambient host environment variables are strictly isolated. Starlark scripts additionally enforce in-VM call gates (`calls.allow` / `calls.deny`) over module APIs.

Grant capabilities with composable CLI flags:

```bash
kite run script.star                                          # deny-all (default: pure compute, isolated env & fs)
kite run script.star --sandbox-path-read=. --sandbox-path-read=/usr/share # repeatable read-only path access
kite run script.star --sandbox-path-write=./dist              # repeatable read-write path access
kite run script.star --sandbox-mount ./data:/data:ro          # read-only bind mount into sandbox destination
kite run script.star --sandbox-mount /opt/app                 # identity bind mount (same host & container path)
kite run script.star --sandbox-mount tmpfs:/scratch           # ephemeral in-memory scratch filesystem
kite run script.star --sandbox-exec-allow=kubectl,helm        # execute specific binaries (auto-resolved on PATH)
kite run script.star --sandbox-net-allow                      # allow outbound network access
kite run script.star --env KUBECONFIG --env ENV=prod          # forward host KUBECONFIG and inject ENV=prod
kite run script.star --sandbox-enabled=false                  # run unconfined
```

Native executables can be confined directly without Starlark VM overhead via `kite exec`:

```bash
kite exec --sandbox-mount ./app:/app:ro --sandbox-mount tmpfs:/tmp -- python3 /app/server.py
```

The five built-in profiles form a capability ladder, each a superset of the prior: `deny-all` ⊂ `sandbox-path` ⊂ `sandbox-exec` ⊂ `sandbox-net` ⊂ `unconfined`.

Define named sandbox profiles in `~/.starkite/config.yaml` or `./config.yaml`:

```yaml
# ~/.starkite/config.yaml
sandbox:
  default:
    paths:
      read: ["."]
  deploy:
    network: host
    paths:
      read: ["."]
      write: ["./build"]
    mounts:
      - src: "./data"
        dst: "/data"
        mode: "ro"
      - type: "tmpfs"
        dst: "/scratch"
    exec:
      allow: ["kubectl", "helm"]
    calls:
      allow: ["k8s.write"]
    env:
      KUBECONFIG:
      DEPLOY_ENV: production
```

Select a profile with `--sandbox-profile=<name>`. See the [sandbox guide](https://starkite.dev/fundamentals/security/sandbox/) for details.

## Error Handling

Every module function has a `try_` variant that returns a `Result` instead of raising:

```python
result = fs.try_read_text("/etc/missing")
if result.ok:
    print(result.value)
else:
    print("Error:", result.error)
```

## Variables

Variables can be injected from CLI flags, YAML files, or environment variables:

```bash
kite script.star --var image=nginx:latest --var-file=prod.yaml
STARKITE_VAR_IMAGE=nginx:latest kite script.star
```

```python
image = var_str("image", "nginx:latest")
replicas = var_int("replicas", 3)
debug = var_bool("debug", False)
```

## Testing

```python
# math_test.star
def test_addition():
    assert_equal(1 + 1, 2)

def test_command():
    result = os.try_exec("echo hello")
    assert_equal(result.ok, True)
```

```bash
kite test ./tests/ --verbose --parallel 4
```

## Examples

See the [`examples/`](examples/) directory for complete working scripts covering system automation, HTTP servers, SSH operations, Kubernetes deployments, and more.

## Documentation

Full documentation is available at [starkite.dev](https://starkite.dev/).

## License

Apache License 2.0

## Contributing

Contributions welcome! Please see CONTRIBUTING.md for guidelines.
