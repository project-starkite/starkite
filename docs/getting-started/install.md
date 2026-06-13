---
title: "Install"
description: "Download a release, build from source, or pull the container image"
weight: 2
---

# Install

Installing Starkite gives you `kite`, the all-in-one binary that bundles every edition's capabilities — command, cloud, and AI modules in one executable. It ships as a single self-contained binary, so there is no runtime to install alongside it and nothing to configure before the first run. Pick the method that matches how you want to manage it: `go install` if you already have a Go toolchain, a prebuilt release if you want bytes you can drop on `PATH`, or a source build when you need the lean editions or a specific commit.

=== "Go Install"

    When you have Go on the machine, `go install` is the shortest path — it fetches, builds, and places the binary in your Go bin directory in one step:

    ```bash
    go install github.com/project-starkite/starkite/kite@latest
    ```

=== "Download from GitHub"

    To skip the toolchain entirely, download a prebuilt binary for your platform from [GitHub Releases](https://github.com/project-starkite/starkite/releases). Reach for the line that matches your OS and architecture:

    ```
    # Linux
    wget https://github.com/project-starkite/starkite/releases/latest/download/kite-linux-amd64 -O kite

    # macOS (Apple Silicon)
    wget https://github.com/project-starkite/starkite/releases/latest/download/kite-darwin-arm64 -O kite

    # Windows (PowerShell)
    Invoke-WebRequest -Uri "https://github.com/project-starkite/starkite/releases/latest/download/kite-windows-amd64.exe" -OutFile kite.exe
    ```

=== "From Source"

    Build from source when you want the lean editions or a specific commit rather than the latest release. The repository is a Go workspace with one module per edition, and `make kite` compiles the default all-in-one binary into `./bin/`:

    ```bash
    git clone https://github.com/project-starkite/starkite.git
    cd starkite
    make kite
    ```

    That leaves the binary under `./bin/`; move it onto `PATH` so you can call `kite` from anywhere:

    ```bash
    sudo install -m 0755 ./bin/kite /usr/local/bin/kite
    ```

    If you want the space-conscious editions as well, `make all` builds all four — the lean `kitecmd` / `kitecloud` / `kiteai` alongside `kite`. Run `make help` to list every target.

## Verify

However you installed it, confirm the binary runs and reports its build:

```bash
kite version
```

The output names the version followed by the commit and build details, which differ per build:

```
kite version v0.1.0
  commit:  <git-sha>
  built:   <timestamp>
  go:      go1.26.1
  os/arch: darwin/arm64
```

---

# Container

If you would rather not place a binary on the host at all, run Starkite from a container. The `kite` binary is published as an OCI image at `ghcr.io/project-starkite/starkite`, built with [ko](https://ko.build/) on a [Chainguard distroless](https://www.chainguard.dev/chainguard-images) base — a minimal image with no shell or package manager, so the attack surface stays small.

For running scripts from the image, see the [Container quickstart](#container).

## Pull the image

Pull `latest` to track the newest release, or pin a tag when you need a reproducible image across machines:

```bash
docker pull ghcr.io/project-starkite/starkite:latest
# or pin to a specific release:
docker pull ghcr.io/project-starkite/starkite:v0.1.0
```
