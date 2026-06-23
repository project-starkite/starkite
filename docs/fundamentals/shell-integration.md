---
title: "Shell integration"
description: "Pipes, environment variables, exit codes, and scripting assertions"
weight: 75
---

# Shell Integration

Starkite operates directly within standard Unix shell environments such as Bash and Zsh. Using command-line evaluation and shebang executions, scripts hook directly into host terminals to extend native pipelines with Starlark runtime capabilities.

The sections below present examples of incorporating Starkite scripting and module features directly into shell workflows.

## System queries

Host properties can be inspected dynamically using the built-in `runtime` module. The Starlark interpreter evaluates these queries directly within the process:

```bash
$ kite exec 'print("OS Platform: " + runtime.platform())'
OS Platform: darwin
```

## Directory globbing

File path matching can be executed directly using the built-in `glob()` function:

```bash
$ kite exec --allow-fs 'print(", ".join(glob("*.md")))'
CHANGELOG.md, README.md
```

## Processing pipeline data (JSON & YAML)

Structured configuration formats are parsed from standard input (`/dev/stdin`) using the built-in `json` and `yaml` modules:

```bash
# Query JSON values inline from a pipeline without installing jq
$ echo '{"env": "production", "status": "active"}' | \
    kite exec --allow-fs 'print(json.decode(read_text("/dev/stdin"))["status"])'
active

# Query local YAML configuration files inline without installing yq
$ kite exec --allow-fs 'print(yaml.decode(read_text("mod.yaml"))["name"])'
```

## Making HTTP API calls

API queries and remote service responses are handled using the built-in `http` module:

```bash
# Fetch details from remote web services without curling or writing external scripts
$ kite exec --allow-net 'print(json.decode(http.url("https://httpbin.org/get").get().body)["url"])'
https://httpbin.org/get
```

## Template rendering

Structured text or configurations are rendered inline using the built-in `template` module:

```bash
# Render templates dynamically with environment variables
$ kite exec --allow-fs 'print(template.render("Hello, {{.user}}!", {"user": env("USER")}))'
Hello, alice!
```

## Cross-platform regex replacements

String search-and-replace patterns are performed using the built-in `regexp` module, maintaining identical syntax rules across macOS and Linux environments:

```bash
# Replace matching digits in system configuration strings
$ kite exec 'print(regexp.replace(r"\d+", "service-v1-port-80", "X"))'
service-vX-port-X
```

## Structured CSV filtering

Tabular datasets are parsed, iterated, and queried directly within pipelines using the built-in `csv` module:

```bash
# Stream and filter CSV records from standard input
$ echo -e "host,status\nweb-1,active\nweb-2,inactive" | \
    kite exec --allow-fs 'print([r["host"] for r in csv.file("/dev/stdin").read(header=True) if r["status"] == "active"])'
["web-1"]
```

## Shell script assertions

Process exit codes returned by `kite exec` can validate inputs and execute assertions inside shell scripts. In Starlark, conditional assertions must reside inside a function body (such as `main()`), and environment lookups require permission profiles that allow filesystem or environment reads (such as `--allow-fs` or `--allow-all`):

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

