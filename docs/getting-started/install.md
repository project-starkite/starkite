---
title: "Install"
description: "Download a release, build from source, or pull the container image"
weight: 2
---

# Install

Starkite ships as a single binary. Download a prebuilt one or build from source.

=== "Go Install"

    Install with `go install`:

    ```bash
    go install github.com/project-starkite/starkite/kite@latest
    ```

=== "Download from GitHub"

    Download a pre-built binary for the target platform from [GitHub Releases](https://github.com/project-starkite/starkite/releases).

    ```
    # Linux
    wget https://github.com/project-starkite/starkite/releases/latest/download/kite-linux-amd64 -O kite

    # macOS (Apple Silicon)
    wget https://github.com/project-starkite/starkite/releases/latest/download/kite-darwin-arm64 -O kite

    # Windows (PowerShell)
    Invoke-WebRequest -Uri "https://github.com/project-starkite/starkite/releases/latest/download/kite-windows-amd64.exe" -OutFile kite.exe
    ```

=== "From Source"

    The repository is a Go workspace with one module per edition. `make kite` builds the default all-in-one binary into `./bin/`:

    ```bash
    git clone https://github.com/project-starkite/starkite.git
    cd starkite
    make kite
    ```

    Move the binary onto `PATH`:

    ```bash
    sudo install -m 0755 ./bin/kite /usr/local/bin/kite
    ```

    To build all four editions (the lean `kitecmd` / `kitecloud` / `kiteai` alongside `kite`), run `make all`. Run `make help` to list every target.

## Verify

```bash
kite version
```

Expected output (commit and Go version differ per build):

```
kite version v0.1.0
  commit:  <git-sha>
  built:   <timestamp>
  go:      go1.26.1
  os/arch: darwin/arm64
```

---

# Container

The `kite` binary is published as an OCI container image at `ghcr.io/project-starkite/starkite`. The image is built with [ko](https://ko.build/) on a [Chainguard distroless](https://www.chainguard.dev/chainguard-images) base.

For running scripts from the image, see the [Container quickstart](#container).

## Pull the image

```bash
docker pull ghcr.io/project-starkite/starkite:latest
# or pin to a specific release:
docker pull ghcr.io/project-starkite/starkite:v0.1.0
```