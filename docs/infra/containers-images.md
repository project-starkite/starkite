---
title: "Image management"
description: "Pull registry images, inspect local caches, and prune unreferenced container resources"
weight: 5
---

# Image management

The `containers` module provides APIs to pull container images from OCI registries, inspect locally cached images, and perform disk housekeeping by pruning unused resources.

## Pulling Images

To pull images from public or private OCI registries, use `client.pull()`:

```python
def main():
    client = containers.config()

    # Pull from public registry
    print("Pulling redis:7-alpine...")
    client.pull("redis:7-alpine")
    print("Pull complete.")
```

### Private Registry Authentication

To pull from private registries (such as GitHub Container Registry, AWS ECR, or Google Artifact Registry), pass the `auth` parameter as a dictionary or a base64-encoded string:

```python
def main():
    client = containers.config()

    # Authenticated pull
    client.pull(
        "ghcr.io/my-org/backend-service:v1.0.0",
        auth = {
            "username": "deploy-agent",
            "password": "ghp_secret_token",
            "serveraddress": "ghcr.io",
        },
    )
```

The engine streams and decodes progress responses line-by-line from the daemon, raising a Starlark error if the pull encounters an authentication failure or missing tag.

---

## Listing Cached Images

Use `client.images()` to inspect all images available locally in the engine cache:

```python
def main():
    client = containers.config()
    images = client.images()

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

## Practical Recipe: Automated Image Update and Housekeeping

This recipe pulls the latest application image, replaces an existing container, and prunes unused images:

```python
def main():
    client = containers.config()
    image_tag = "redis:7-alpine"

    print("Pulling latest image:", image_tag)
    client.pull(image_tag)

    # Stop and remove existing container if running
    for c in client.list(all=True):
        if c.name == "app-cache":
            print("Stopping existing container instance...")
            client.stop(c, timeout = 5)
            client.delete(c, force = True)
            break

    # Deploy new container instance
    print("Deploying updated container...")
    new_c = client.run(
        image = image_tag,
        name = "app-cache",
        ports = {"6379/tcp": 6379},
        detach = True,
    )
    print("Updated container running with ID:", new_c.id[:12])

    # Clean up dangling images
    prune_res = client.prune(containers=False, images=True)
    print("Reclaimed %d bytes from dangling images" % prune_res["space_reclaimed"])
```

Run with:
```bash
kite run ./update_service.star --permissions=allow-local
```

---

## Permissions

* `containers.read`: Required for `client.images()`.
* `containers.write`: Required for `client.pull()` and `client.prune()`.

Both capabilities are granted by default under `--permissions=allow-local`.
