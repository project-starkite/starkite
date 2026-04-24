---
title: "Getting Started"
description: "Install starkite and write your first script"
weight: 1
---

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/project-starkite/starkite.git
cd starkite

# Build the base edition
go build -o kite .

# Build the cloud edition (includes Kubernetes modules)
go build -o kite-cloud ./cmd/cloud/starkite/
```

### From GitHub Releases

Download the latest binary for your platform from [GitHub Releases](https://github.com/project-starkite/starkite/releases).

Available binaries:
- `kite-linux-amd64`, `kite-linux-arm64`
- `kite-darwin-amd64`, `kite-darwin-arm64`
- `kite-windows-amd64.exe`
- Cloud editions: `kite-cloud-*` (same platforms)

### Via `go install`

```bash
go install github.com/project-starkite/starkite@latest
```

## Verify Installation

```bash
kite version
# Output: kite version 0.0.1 (base)
```

## Your First Script

Create a file called `hello.star`:

```python
#!/usr/bin/env kite
# hello.star — Your first starkite script

name = var_str("name", "World")
print("Hello, " + name + "!")

# Use built-in modules
info = runtime.uname()
log.info("Running on", platform=runtime.platform(), arch=runtime.arch())
```

Run it:

```bash
kite hello.star
kite hello.star --var name=Alice
```

## What's Next?

- [CLI Reference](../cli/index.md) — All available commands
- [Module Reference](../modules/index.md) — 25+ built-in modules
- [Variables](../guides/variables.md) — Variable injection system
- [Error Handling](../guides/error-handling.md) — The `try_` pattern
