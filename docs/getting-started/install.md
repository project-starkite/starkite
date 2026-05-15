---
title: "Install"
description: "Build from source, download a release, or pull the container image"
weight: 2
---

# Install

Starkite ships as a single, dependency-free binary per edition. Pick a path that fits the host.

=== "From Source"

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

---

# Container

The all-in-one `kite` binary is published as an OCI container image at `ghcr.io/project-starkite/kite`. The image is built with [ko](https://ko.build/) on a [Chainguard distroless](https://www.chainguard.dev/chainguard-images) base, signed with [cosign](https://docs.sigstore.dev/cosign/) keyless via GitHub OIDC, and ships with an SPDX SBOM as an OCI referrer.

For running scripts from the image, see the [Container quickstart](quickstarts/container.md).

## Pull the image

```bash
docker pull ghcr.io/project-starkite/kite:latest
# or pin to a specific release:
docker pull ghcr.io/project-starkite/kite:v0.1.0
```

The multi-arch manifest covers `linux/amd64` and `linux/arm64`. Docker picks the matching variant automatically.

## Verify the signature

Every published image is signed with cosign keyless. The certificate's identity binds the image to the release workflow:

```bash
cosign verify ghcr.io/project-starkite/kite:v0.1.0 \
  --certificate-identity-regexp="^https://github.com/project-starkite/starkite/.github/workflows/release\.yml@refs/" \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

A successful verification proves the image was built by the official release workflow at the corresponding tag.

## Inspect the SBOM

The SPDX SBOM is published as an OCI referrer alongside the image:

```bash
cosign download sbom ghcr.io/project-starkite/kite:v0.1.0 > kite.spdx.json
```

## Extend the image

The base is distroless (no shell, no package manager, no `apt`/`apk`/`yum`). To add tools — for example, `kubectl` for cloud workflows — bundle them via a multi-stage build:

```dockerfile
# syntax=docker/dockerfile:1.6
FROM ghcr.io/project-starkite/kite:v0.1.0 AS kite

FROM cgr.dev/chainguard/static:latest
COPY --from=kite /ko-app/kite /usr/local/bin/kite
COPY --from=alpine/k8s:1.32.0 /usr/bin/kubectl /usr/local/bin/kubectl
ENTRYPOINT ["/usr/local/bin/kite"]
```

For runtime-only Starlark module additions (no image rebuild), use `kite module install` inside the container.

## Image labels

Images include the standard OCI `org.opencontainers.image.*` labels (source, revision, version) populated by ko at build time. Inspect with:

```bash
docker inspect ghcr.io/project-starkite/kite:v0.1.0 \
  | jq '.[0].Config.Labels'
```
