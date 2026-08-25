<p align="center">
    <img src="./docs/assets/images/starkite-logo-banner-bg.png" alt="Starkite" width="550">
    <h3 align="center"> Secure Runtime for Cloud-Native and Agentic AI Automation with Starlark </h3> 
</p>

**starkite** is an automation language built on [Starlark](https://github.com/google/starlark-go) (a Python-like language). It exposes Go's standard library as type-safe, scriptable Starlark modules — providing a unified interface for general-purpose, cloud-native, and GenAI agent automation.

## Features

- **Go stdlib as modules** — Type-safe, scriptable access to Go's standard library
- **Python-like syntax** — Uses Starlark, a deterministic, hermetic Python dialect
- **General-purpose automation** — System tasks, scripting, data processing
- **Cloud-native operations** — Kubernetes integration, infrastructure management (Cloud edition)
- **GenAI agent automation** — Tool execution and orchestration for AI agents
- **27+ built-in modules** — OS, filesystem, HTTP (client + server), SSH, JSON, YAML, CSV, concurrency, retry, and more
- **SSH operations** — Multi-host concurrent execution, jump hosts, SCP upload/download
- **Resilience patterns** — Retry with exponential backoff, concurrent map/each/exec
- **Safe by default** — Permission sandboxing with fine-grained allow/deny rules
- **Built-in test runner** — Test framework with assertions, filtering, and setup/teardown

## Editions

The default binary is **`kite`**, the all-in-one edition — it bundles every module and is what all examples and documentation use. The lean editions are smaller subsets of the same binary, for space-conscious targets.

| Binary | Adds on top of base | Use when |
|---|---|---|
| `kite` | base + Kubernetes + GenAI/MCP (all-in-one) | you want everything in one binary |
| `kitecmd` | base modules only (os, fs, http, ssh, json, yaml, time, log, …) | system scripts, CI tasks, general automation |
| `kitecloud` | base + Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kiteai` | base + LLM clients + MCP server/client | agentic AI tools and orchestration |

Use `kite` unless binary size or attack surface is a real constraint — init containers, edge nodes, or CI runners under a restricted profile such as `--permissions=allow-fs`. The lean editions (`kitecmd` / `kitecloud` / `kiteai`) are a strict subset for those targets.

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
  curl -fsSL https://install.starkite.run | sh
  ```

* **Windows (PowerShell Installer Script)**:
  ```powershell
  irm https://install.starkite.run/install.ps1 | iex
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
docker run --rm ghcr.io/project-starkite/starkite:latest exec 'print(hostname())'
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
make build-ai           # ./bin/kiteai     (base + LLM/MCP)
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
print(result["stdout"])
```

Then, make the script executable and run the script:

```bash
chmod +x hello.star
./hello.star --var name=Alice
```

Or, run it directly with `kite`:

```bash
kite hello.star
kite hello.star --var name=Alice
```

## CLI

```bash
kite run script.star                  # Execute a script
kite exec 'print(os.hostname())'      # Inline execution
kite repl                             # Interactive REPL
kite test ./tests/                    # Run tests
kite watch script.star                # Re-run on file changes
kite validate script.star             # Syntax check
kite module install <repo>            # Install a module
kite update                           # Self-update
```

See the [CLI reference](https://starkite.ai/references/cli/) for all commands and flags.

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
client = ssh.config(user="admin", host_list=["web1", "web2", "web3"])
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
| AI | `ai`, `mcp` (in `kite` and `kiteai`) |

See the [API reference](https://starkite.ai/references/api/) for full module documentation.

## Permission Sandbox

starkite controls script privileges via CLI flags. The default is `deny-all` — a script runs with pure compute plus `print`/`log` until granted more:

```bash
kite run script.star                          # deny-all (default)
kite run script.star --permissions=allow-fs   # read any file; write within $CWD; env
kite run script.star --permissions=allow-local # serve, $CWD exec, k8s, ai
kite run script.star --permissions=allow-all  # unrestricted
```

The five built-in profiles form a capability ladder, each a superset of the prior: `deny-all` ⊂ `allow-fs` ⊂ `allow-net` ⊂ `allow-local` ⊂ `allow-all`.

For fine-grained control, define named profiles in `config.yaml` and select one with `--permissions=<name>`:

```yaml
# ~/.starkite/config.yaml
permissions:
  default: { allow: ["fs.read", "http.client"] }   # this machine's everyday ceiling
  deploy:
    allow:
      - fs.read($CWD/**)
      - http.client
      - k8s.write
    deny:
      - os.exec
```

Each profile is allow-list only; `deny` rules carve out exceptions and take precedence. An unspecified `--permissions` resolves to the `default` profile if defined, else `deny-all`. See the [permissions reference](https://starkite.ai/fundamentals/security/permissions/) for details.

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
    result = os.exec("echo hello")
    assert_equal(result["exit_code"], 0)
```

```bash
kite test ./tests/ --verbose --parallel 4
```

## Examples

See the [`examples/`](examples/) directory for complete working scripts covering system automation, HTTP servers, SSH operations, Kubernetes deployments, and more.

## Documentation

Full documentation is available at [starkite.ai](https://starkite.ai/).

## License

Apache License 2.0

## Contributing

Contributions welcome! Please see CONTRIBUTING.md for guidelines.
