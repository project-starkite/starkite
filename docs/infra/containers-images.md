---
title: "Image management"
description: "Pull registry images, build from local context, inspect metadata, list caches, and prune unreferenced container resources"
weight: 5
---

# Image management

The `containers` module provides APIs to pull container images from OCI registries, build images from local directory contexts, inspect image metadata, list locally cached images, remove images, and perform disk housekeeping by pruning unused resources.

### Method Naming and Aliases

Image operations on the container client use explicit `image_*` prefixes to distinguish image actions from container lifecycle methods (`run`, `stop`, `exec`, `list`, `delete`):

| Canonical Method | Short Alias | Description |
|:---|:---|:---|
| `client.image_pull(image, auth=None)` | `client.pull()` | Pull an image from an OCI registry. |
| `client.image_build(path, tag=None, dockerfile="Dockerfile")` | `client.build()` | Build an image from a local context directory. |
| `client.image_list(all=False)` | `client.images()` | List images present in the local engine cache. |
| `client.image_inspect(image)` | — | Retrieve low-level metadata for an image. |
| `client.image_remove(image, force=False)` | `client.rmi()` | Remove an image from the local cache. |

Both canonical and alias forms are also exposed as top-level shortcuts on the `containers` module (for example, `containers.image_pull()` and `containers.pull()`).

---

## Pulling Images

To pull an image from a public or private OCI registry, call `client.image_pull()` (or alias `client.pull()`):

```python
def main():
    client = containers.config()

    # Pull from public registry
    print("Pulling redis:7-alpine...")
    client.image_pull("redis:7-alpine")
    print("Pull complete.")
```

### Private Registry Authentication

To authenticate with private registries (such as GitHub Container Registry, AWS ECR, or Google Artifact Registry), supply the `auth` parameter as a dictionary or a base64-encoded string:

```python
def main():
    client = containers.config()

    # Authenticated pull
    client.image_pull(
        "ghcr.io/my-org/backend-service:v1.0.0",
        auth = {
            "username": "deploy-agent",
            "password": "ghp_secret_token",
            "serveraddress": "ghcr.io",
        },
    )
```

The daemon streams and decodes progress responses line-by-line, raising a Starlark error if authentication fails or the requested tag does not exist.

---

## Building Images

Use `client.image_build()` (or alias `client.build()`) to build an image from a local context directory containing a Dockerfile:

```python
def main():
    client = containers.config()

    # Build an image from local directory context
    logs = client.image_build(
        path = "./app",
        tag = "my-service:v1.0.0",
        dockerfile = "Dockerfile",
    )
    print(logs)
```

### Build Parameters

`client.image_build(path, tag=None, dockerfile="Dockerfile")`:

* `path` (*str* or *fs.path*, required): Local directory path containing the build context. The runtime automatically packages the directory into an in-memory tar archive and streams it to the engine REST API.
* `tag` (*str*, optional): Repository name and optional tag to apply upon successful build.
* `dockerfile` (*str*, default `"Dockerfile"`): Path to the Dockerfile relative to the context directory.

The function returns the formatted build output logs captured from the engine build stream.

---

## Inspecting Images

Use `client.image_inspect()` to retrieve detailed metadata for an image:

```python
def main():
    client = containers.config()
    info = client.image_inspect("redis:7-alpine")

    print("Image ID:", info["Id"])
    print("Architecture:", info.get("Architecture"))
    print("Size (bytes):", info.get("Size"))
    print("Tags:", info.get("RepoTags"))
```

`client.image_inspect()` returns a structured dictionary matching the daemon's image inspect JSON schema, including architecture, OS, layer digests, environment variables, and default command specifications.

---

## Listing Cached Images

Use `client.image_list()` (or alias `client.images()`) to inspect all images available locally in the engine cache:

```python
def main():
    client = containers.config()
    images = client.image_list()

    print("Found %d cached images:" % len(images))
    for img in images:
        tags = ", ".join(img["repo_tags"]) if img["repo_tags"] else "<untagged>"
        size_mb = img["size"] / (1024 * 1024)
        print("  - ID: %s | Size: %.1f MB | Tags: %s" % (img["id"][:12], size_mb, tags))
```

### Image Dictionary Fields

* `id` (*str*): SHA identifier of the image.
* `repo_tags` (*list[str]*): Tag references associated with this image.
* `repo_digests` (*list[str]*): Digest hashes of the image layers.
* `size` (*int*): Total virtual disk size in bytes.
* `created` (*int*): Unix epoch timestamp of image creation.
* `labels` (*dict[str, str]*): Key-value labels embedded in the image manifest.

---

## Removing Images

Use `client.image_remove()` (or alias `client.rmi()`) to delete an image from the local cache:

```python
def main():
    client = containers.config()

    # Remove an image by tag
    client.image_remove("my-service:v1.0.0")

    # Force remove an image currently referenced by stopped containers
    client.image_remove("redis:7-alpine", force = True)
```

---

## Resource Housekeeping and Pruning

Over time, stopped containers, build layers, and untagged images accumulate disk space. The `client.prune()` function provides structured resource pruning:

```python
def main():
    client = containers.config()

    # Prune stopped containers and dangling images
    report = client.prune(
        containers = True,
        images = True,
        volumes = False,
    )

    print("Containers removed:", len(report["containers_deleted"]))
    print("Images removed:", len(report["images_deleted"]))
    mb_reclaimed = report["space_reclaimed"] / (1024 * 1024)
    print("Disk space reclaimed: %.2f MB" % mb_reclaimed)
```

### Prune Parameters

`client.prune(containers=True, volumes=False, images=False)`:

| Parameter | Type | Default | Description |
|:---|:---|:---|:---|
| `containers` | `bool` | `True` | Remove all stopped containers. |
| `volumes` | `bool` | `False` | Remove unreferenced volumes. |
| `images` | `bool` | `False` | Remove dangling (untagged) images. |

### Prune Report Fields

The returned dictionary contains:
* `containers_deleted` (*list[str]*): List of deleted container IDs.
* `images_deleted` (*list[str]*): List of deleted image IDs or tag references.
* `volumes_deleted` (*list[str]*): List of removed volume names.
* `space_reclaimed` (*int*): Total bytes of disk space freed.

---

## Practical Recipe: Automated Build, Deploy, and Cleanup

This recipe builds an application image from a local directory, stops any existing instance, launches the new version, and cleans up dangling build layers:

```python
def main():
    client = containers.config()
    image_tag = "my-app:local"

    print("Building application image...")
    client.image_build(path = "./app", tag = image_tag)

    # Stop and remove existing container if running
    for c in client.list(all = True):
        if c.name == "app-instance":
            print("Stopping existing container...")
            client.stop(c, timeout = 5)
            client.delete(c, force = True)
            break

    # Deploy new container instance
    print("Deploying container instance...")
    c = client.run(
        image = image_tag,
        name = "app-instance",
        ports = {"8080/tcp": 8080},
        detach = True,
    )
    print("Container running with ID:", c.id[:12])

    # Clean up dangling images
    prune_res = client.prune(containers = False, images = True)
    print("Reclaimed %d bytes from dangling images" % prune_res["space_reclaimed"])
```

Execute the script with:
```bash
kite run ./deploy.star --permissions=allow-local
```

---

## Permissions

* `containers.read`: Required for `client.image_list()` and `client.image_inspect()`.
* `containers.write`: Required for `client.image_pull()`, `client.image_build()`, `client.image_remove()`, and `client.prune()`.

Both capabilities are granted by default under `--permissions=allow-local`.
