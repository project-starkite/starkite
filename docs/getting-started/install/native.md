---
title: "Install (native)"
description: "Build from source or download a pre-built binary"
weight: 1
---

Starkite ships as a single, dependency-free binary per edition.

=== "From source"

    The repository is a Go workspace with one module per edition. Build the editions needed — local builds land in `./bin/`:

    ```bash
    git clone https://github.com/project-starkite/starkite.git
    cd starkite

    make build              # all four binaries → ./bin/
    # or:
    make build-all          # ./bin/kite       (all-in-one)
    make build-base         # ./bin/kitecmd    (base only)
    make build-cloud        # ./bin/kitecloud  (base + k8s)
    make build-ai           # ./bin/kiteai     (base + LLM/MCP)
    ```

    Move the binary onto `PATH`:

    ```bash
    sudo install -m 0755 ./bin/kite /usr/local/bin/kite
    ```

=== "From GitHub Releases"

    Download a pre-built binary for the target platform from [GitHub Releases](https://github.com/project-starkite/starkite/releases).

    Release assets follow the `<binary>-<os>-<arch>` pattern:

    - `kite-linux-amd64`, `kite-linux-arm64`, `kite-darwin-amd64`, `kite-darwin-arm64`, `kite-windows-amd64.exe`
    - `kitecmd-*`, `kitecloud-*`, `kiteai-*` (same OS/arch matrix)

    Rename the downloaded file to `kite` (or `kitecmd` / `kitecloud` / `kiteai`), make it executable, and place it on `PATH`.

## Verify

```bash
kite version
```

Expected output (commit and Go version differ per build):

```
kite version v0.1.0 (all)
  edition: all
  commit:  <git-sha>
  built:   <timestamp>
  go:      go1.26.1
  os/arch: darwin/arm64
```

`kitecmd version` reports `(base)`, `kitecloud version` reports `(cloud)`, `kiteai version` reports `(ai)`.
