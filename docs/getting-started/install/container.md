---
title: "Container Distribution"
description: "Running starkite as a container image"
weight: 9
---

The all-in-one `kite` binary is published as an OCI container image at
`ghcr.io/project-starkite/kite`. The image is built with [ko](https://ko.build/)
on a [Chainguard distroless](https://www.chainguard.dev/chainguard-images)
base, signed with [cosign](https://docs.sigstore.dev/cosign/) keyless via
GitHub OIDC, and ships with an SPDX SBOM as an OCI referrer.

## Pull the image

```bash
docker pull ghcr.io/project-starkite/kite:latest
# or pin to a specific release:
docker pull ghcr.io/project-starkite/kite:v0.1.0
```

Multi-arch manifest covers `linux/amd64` and `linux/arm64`. Docker selects
the right one automatically.

## Run a script

The image's entrypoint is the `kite` binary, so subcommands flow through directly:

```bash
# One-liner via `kite exec`
docker run --rm ghcr.io/project-starkite/kite:latest \
  exec 'print("hello from " + hostname())'

# Mount a script directory and run a file
docker run --rm \
  -v "$PWD:/work:ro" -w /work \
  ghcr.io/project-starkite/kite:latest \
  run my-script.star

# Run tests under the strict permission profile
docker run --rm \
  -v "$PWD:/work:ro" -w /work \
  ghcr.io/project-starkite/kite:latest \
  test --permissions=strict tests/
```

## Run on Kubernetes

A one-shot job:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kite-run
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: kite
          image: ghcr.io/project-starkite/kite:v0.1.0
          args: ["run", "/scripts/main.star"]
          volumeMounts:
            - name: scripts
              mountPath: /scripts
              readOnly: true
      volumes:
        - name: scripts
          configMap:
            name: my-scripts
```

`kubectl run --rm -it kite -- exec 'print("hi")' --image=ghcr.io/project-starkite/kite:latest`
also works for one-off invocations.

## Verify the signature

Every published image is signed with cosign keyless. The certificate's
identity binds the image to this repository's release workflow:

```bash
cosign verify ghcr.io/project-starkite/kite:v0.1.0 \
  --certificate-identity-regexp="^https://github.com/project-starkite/starkite/.github/workflows/release\.yml@refs/" \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

A successful verification proves the image was built by the official release
workflow at the corresponding tag.

## Inspect the SBOM

The SPDX SBOM is published as an OCI referrer alongside the image:

```bash
cosign download sbom ghcr.io/project-starkite/kite:v0.1.0 > kite.spdx.json
```

## Extending the image

The base is distroless (no shell, no package manager, no `apt`/`apk`/`yum`).
To add tools — for example, `kubectl` for cloud workflows — bundle them via
a multi-stage build:

```dockerfile
# syntax=docker/dockerfile:1.6
FROM cgr.dev/chainguard/static:latest AS tools
# (no-op stage to anchor the base)

FROM ghcr.io/project-starkite/kite:v0.1.0 AS kite

FROM cgr.dev/chainguard/static:latest
COPY --from=kite /ko-app/kite /usr/local/bin/kite
COPY --from=alpine/k8s:1.32.0 /usr/bin/kubectl /usr/local/bin/kubectl
ENTRYPOINT ["/usr/local/bin/kite"]
```

Or, if you only need additional Starlark modules, install them at runtime
with `kite module install`. No image rebuild required.

## Image labels

Images include the standard OCI `org.opencontainers.image.*` labels (source,
revision, version) — populated by ko at build time. Inspect with:

```bash
docker inspect ghcr.io/project-starkite/kite:v0.1.0 \
  | jq '.[0].Config.Labels'
```
