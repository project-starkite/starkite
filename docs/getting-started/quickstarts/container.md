---
title: "Run scripts from the container image"
description: "Use ghcr.io/project-starkite/kite to run scripts without installing kite"
weight: 8
---

# Run scripts from the container image

The all-in-one `kite` binary is published as `ghcr.io/project-starkite/kite`. The image's entrypoint is `kite`, so subcommands flow through directly. For pulling and verifying the image, see [Install > Container](../install.md#container).

## One-liner via kite exec

```bash
docker run --rm ghcr.io/project-starkite/kite:latest \
  exec 'print("hello from " + hostname())'
```

## Mount a script directory and run a file

```bash
docker run --rm \
  -v "$PWD:/work:ro" -w /work \
  ghcr.io/project-starkite/kite:latest \
  run my-script.star
```

The `:ro` mount keeps the host directory read-only; drop it if the script needs to write.

## Run tests under a strict permission profile

```bash
docker run --rm \
  -v "$PWD:/work:ro" -w /work \
  ghcr.io/project-starkite/kite:latest \
  test --permissions=strict tests/
```

## What's happening

- The image is multi-arch (`linux/amd64`, `linux/arm64`); Docker picks the matching variant automatically.
- The base is distroless — no shell, no package manager. To add tools, see [Extend the image](../install.md#extend-the-image).
- For Kubernetes-native execution, see the [Kubernetes quickstart](kubernetes.md).

## See also

- [Install > Container](../install.md#container) — pull, verify signature, inspect SBOM
- [Permissions quickstart](permission.md) — the `--permissions` flag used above
