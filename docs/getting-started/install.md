---
title: "Install"
description: "Download a release, build from source, or pull the container image"
weight: 2
---

# Install

Starkite is distributed as a single, self-contained binary (`kite`) that bundles the command, cloud, and AI automation modules. It requires no external runtime or pre-run configuration. Select an installation method below to install `kite` on your system, or pull the container image to run scripts without local binaries.

=== "macOS"

    Install starkite on macOS using the Homebrew package manager:

    ```bash
    brew install project-starkite/tap/kite
    ```

=== "Linux"

    Use the installer script to automatically download the prebuilt binary for your platform and CPU architecture.

    To install to the default location (`/usr/local/bin/kite`):

    ```bash
    curl -fsSL https://starkite.run/install.sh | sh
    ```

    To install to a custom directory (e.g. `~/.local/bin`) without requiring root/sudo privileges:

    ```bash
    curl -fsSL https://starkite.run/install.sh | PREFIX=~/.local/bin sh
    ```

=== "Windows"

    Install starkite on Windows using the Scoop package manager:

    ```powershell
    scoop bucket add starkite https://github.com/project-starkite/scoop-bucket
    scoop install kite
    ```

## Download Binary

Download the prebuilt binary for your operating system and architecture from [GitHub Releases](https://github.com/project-starkite/starkite/releases):

```bash
# Linux (amd64)
curl -Lo kite https://github.com/project-starkite/starkite/releases/latest/download/kite-linux-amd64

# macOS (Apple Silicon)
curl -Lo kite https://github.com/project-starkite/starkite/releases/latest/download/kite-darwin-arm64

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/project-starkite/starkite/releases/latest/download/kite-windows-amd64.exe" -OutFile kite.exe
```

After downloading, make the binary executable and move it to a directory in your system `PATH`:

```bash
chmod +x kite
sudo mv kite /usr/local/bin/
```

## Go Install

If you have a Go toolchain installed, run `go install` to download and compile `kite` into your `GOBIN` directory:

```bash
go install github.com/project-starkite/starkite/kite@latest
```

## From Source

To compile a specific commit or build specialized editions, clone the repository and run the make commands. The repository is configured as a Go workspace with one module per edition:

```bash
git clone https://github.com/project-starkite/starkite.git
cd starkite
make kite
```

This creates the all-in-one binary at `./bin/kite`. Move the binary to a directory in your system `PATH`:

```bash
sudo install -m 0755 ./bin/kite /usr/local/bin/kite
```

To build the separate, specialized editions (`kitecmd`, `kitecloud`, and `kiteai`) in addition to the unified `kite` binary, run:

```bash
make all
```

Run `make help` to list all available compilation targets.

## Verify

Verify the installation by running:

```bash
kite version
```

The output displays the release version, commit hash, build timestamp, and target architecture:

```
kite version v0.1.0
  commit:  <git-sha>
  built:   <timestamp>
  go:      go1.26.1
  os/arch: darwin/arm64
```

---

# Container Image

Starkite is available as an OCI container image at `ghcr.io/project-starkite/starkite`. The image is built using [ko](https://ko.build/) on a minimal [Chainguard distroless](https://www.chainguard.dev/chainguard-images) base.

## Pull the Image

Pull the latest release or pin to a specific tag:

```bash
# Pull the latest release
docker pull ghcr.io/project-starkite/starkite:latest

# Or pull a specific version
docker pull ghcr.io/project-starkite/starkite:v0.1.0
```

## Run Scripts in a Container

To execute a local script using the container image, mount your working directory and pass the script filename:

```bash
docker run --rm -v "$(pwd)":/work -w /work ghcr.io/project-starkite/starkite:latest run ./hello.star
```
