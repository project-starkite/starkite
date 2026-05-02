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
- **WASM extensible** — Extend with WebAssembly plugins written in Go, Rust, or any WASM-compatible language
- **Built-in test runner** — Test framework with assertions, filtering, and setup/teardown

## Editions

Starkite ships as four independent binaries that share the same script language and core modules. Pick the one that matches what you want to automate.

| Binary | Adds on top of base | Use when |
|---|---|---|
| `kite` | base + Kubernetes + GenAI/MCP (all-in-one) | you want everything in one binary |
| `kitecmd` | base modules only (os, fs, http, ssh, json, yaml, time, log, …) | system scripts, CI tasks, general automation |
| `kitecloud` | base + Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kiteai` | base + LLM clients + MCP server/client | agentic AI tools and orchestration |

`kite` is the recommended starter — it's a strict superset of the lean editions. Install `kitecmd` if you want a smaller binary or smaller attack surface under `--permissions=strict`.

> **Naming convention.** Source directories follow two shapes: infrastructure packages (`libkite/`, `allkite/`) use a `<descriptor>kite/` suffix form; domain editions (`base/`, `cloud/`, `ai/`) are bare nouns. Binaries use `kite<edition>` prefix form (`kitecmd`, `kitecloud`, `kiteai`), with the all-in-one as the unadorned `kite`.

## Installation

Download pre-built binaries from [GitHub Releases](https://github.com/project-starkite/starkite/releases). Release assets follow the `<binary>-<os>-<arch>` pattern: `kite-linux-amd64`, `kitecmd-darwin-arm64`, `kitecloud-windows-amd64.exe`, etc.

Or build from source — the repository is a Go workspace with one module per edition:

```bash
git clone https://github.com/project-starkite/starkite.git
cd starkite

make build              # builds all four binaries into ./bin/
# or:
make build-all          # ./bin/kite       (all-in-one)
make build-base         # ./bin/kitecmd   (base only)
make build-cloud        # ./bin/kitecloud  (base + k8s)
make build-ai           # ./bin/kiteai     (base + LLM/MCP)
```

Move the binary onto your `PATH`:

```bash
sudo install -m 0755 ./bin/kite /usr/local/bin/kite
```

Or run it as a container — the all-in-one `kite` is published as a signed
distroless image:

```bash
docker run --rm ghcr.io/project-starkite/kite:latest exec 'print(hostname())'
```

See [Container Distribution](docs/guides/container-distribution.md) for
multi-arch details, signature verification, and Kubernetes usage.

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

See the [CLI reference](https://starkite.dev/cli/) for all commands and flags.

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

See the [module reference](https://starkite.dev/modules/) for full API documentation.

## Permission Sandbox

starkite controls script privileges via CLI flags:

```bash
kite run script.star                        # Trust mode (default): allow all operations
kite run script.star --permissions=strict   # Restrict to safe operations only
```

The `strict` profile blocks command execution, file I/O, and network access — only safe operations like string manipulation, JSON/YAML encoding, and math are allowed.

For fine-grained control, configure allow/deny rules in `config.yaml`:

```yaml
# config.yaml
project:
  name: my-project

permissions:
  default: deny
  allow:
    - "fs.read_text(./data/**)"
    - "http.get"
    - "strings.*"
    - "json.*"
  deny:
    - "os.exec"
    - "ssh.*"
```

Deny rules take precedence over allow rules. Modules enforce permissions internally — a sandboxed script that calls `fs.write_file()` without a matching allow rule gets a permission error. See the [permissions guide](https://starkite.dev/guides/permissions/) for details.

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

Full documentation is available at [starkite.dev](https://starkite.dev/).

## License

Apache License 2.0

## Contributing

Contributions welcome! Please see CONTRIBUTING.md for guidelines.
