# starkite — Automation Language

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

## Installation

```bash
# Install via go
go install github.com/project-starkite/starkite@latest

# Or build from source
git clone https://github.com/project-starkite/starkite.git
cd starkite && go build -o kite .
```

Download pre-built binaries from [GitHub Releases](https://github.com/project-starkite/starkite/releases).

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
| Cloud | `k8s` (Cloud edition) |

See the [module reference](https://starkite.dev/modules/) for full API documentation.

## Permission Sandbox

starkite has two permission modes controlled via CLI flags:

```bash
kite run script.star --trust     # Allow all operations (default)
kite run script.star --sandbox   # Restrict to safe operations only
```

Sandbox mode blocks command execution, file I/O, and network access — only safe operations like string manipulation, JSON/YAML encoding, and math are allowed.

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
