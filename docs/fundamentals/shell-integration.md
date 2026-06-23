---
title: "Shell integration"
description: "Pipes, environment variables, exit codes, and scripting assertions"
weight: 75
---

# Shell Integration

Starkite is designed to integrate cleanly with standard Unix shells like Bash and Zsh. Because the `kite` binary is self-contained and pre-packages modules for filesystem operations, network requests, and structural format parsing, it can augment standard shell pipelines without requiring external CLI dependencies.

This guide illustrates common patterns for integrating `kite exec` and shebang-enabled scripts directly into your terminal workflows.

## System queries

You can inspect host properties dynamically using built-in runtime functions. Since these are evaluated directly by the Starlark interpreter, they run securely without process-forking overhead or requiring system permissions:

```bash
$ kite exec 'print("OS Platform: " + runtime.platform())'
OS Platform: darwin
```

## Directory globbing

Starkite provides built-in path matching functions to scan and list files directly:

```bash
$ kite exec --allow-fs 'print(", ".join(glob("*.md")))'
CHANGELOG.md, README.md
```

## Processing pipeline data (JSON & YAML)

You can decode structured configurations or stream pipeline data using `yaml.decode()` and `json.decode()` combined with filesystem standard inputs (`/dev/stdin`):

```bash
# Query JSON values inline from a pipeline without installing jq
$ echo '{"env": "production", "status": "active"}' | \
    kite exec --allow-fs 'print(json.decode(read_text("/dev/stdin"))["status"])'
active

# Query local YAML configuration files inline without installing yq
$ kite exec --allow-fs 'print(yaml.decode(read_text("mod.yaml"))["name"])'
```

## Making HTTP API calls

Execute API queries and inspect remote service responses using the built-in `http` module:

```bash
# Fetch details from remote web services without curling or writing external scripts
$ kite exec --allow-net 'print(json.decode(http.url("https://httpbin.org/get").get().body)["url"])'
https://httpbin.org/get
```

## Template rendering

Render structured text or HTML configurations inline using local parameters, eliminating the need to install external templating CLI engines (like `gomplate` or `jinja-cli`):

```bash
# Render templates dynamically with environment variables
$ kite exec --allow-fs 'print(template.render("Hello, {{.user}}!", {"user": env("USER")}))'
Hello, alice!
```

## Cross-platform regex replacements

Perform string search-and-replace patterns using Go's standard regular expression engine, avoiding regex dialect and compatibility differences between macOS (`sed` BSD) and Linux (`sed` GNU):

```bash
# Replace matching digits in system configuration strings
$ kite exec 'print(regexp.replace(r"\d+", "service-v1-port-80", "X"))'
service-vX-port-X
```

## Structured CSV filtering

Parse, iterate, and query tabular dataset records directly inside your shell pipelines without installing heavy spreadsheet libraries (like `pandas`) or dedicated CSV tools:

```bash
# Stream and filter CSV records from standard input
$ echo -e "host,status\nweb-1,active\nweb-2,inactive" | \
    kite exec --allow-fs 'print([r["host"] for r in csv.file("/dev/stdin").read(header=True) if r["status"] == "active"])'
["web-1"]
```

## Shell script assertions

Use the process exit codes returned by `kite exec` to validate inputs and perform assertion checks inside standard shell scripts. In Starlark, conditional checks must reside inside a function body (like `main()`), and environment variables require reading permissions (`--allow-fs` or `--allow-all`):

```bash
# Validate environment port configurations in a Bash conditional wrapper
if PORT=8080 kite exec --allow-fs '
def main():
    if int(env("PORT", "80")) < 1024:
        fail("Privileged port configuration")
' 2>/dev/null; then
    echo "Valid port configuration."
else
    echo "Configuration Error: Port must be 1024 or higher."
fi
```
