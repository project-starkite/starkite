---
title: "File transfers"
description: "Upload and download files securely across remote hosts using Starkite"
weight: 45
---

# File transfers

The `ssh` module supports secure file transfers over SSH using the native Secure Copy Protocol (SCP). The runtime provides `client.upload()` and `client.download()` functions to transfer files between your local system and remote hosts concurrently, managing connection reuse and parallel execution automatically.

---

## Uploading Files

To upload a local file to one or more remote hosts, use the `client.upload()` method. This function allows you to specify the source path, the remote destination path, and the file permission mode.

```python
def deploy_application_config():
    # Configure the SSH client
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        auth  = {
            "user": "alice",
            "key":  "~/.ssh/id_ed25519",
        },
    )
    
    # Upload a local config file and set its permission mode to 0640
    results = client.upload(
        src = "configs/app.conf",
        dst = "/etc/alice-app/app.conf",
        mode = "0640",
    )
    
    # Process results for each host
    for r in results:
        if r.ok:
            print(r.host, "successfully uploaded:", r.bytes, "bytes")
        else:
            log.error("Upload failed", {"host": r.host, "error": r.dst})
```

---

## Downloading Files

To retrieve files from remote hosts, use the `client.download()` method. You specify the remote source file and the local destination directory.

```python
def fetch_remote_logs():
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        auth  = {
            "user": "alice",
            "key":  "~/.ssh/id_ed25519",
        },
    )
    
    # Download remote logs to a local directory
    results = client.download(
        src = "/var/log/alice-app/error.log",
        dst = "./logs/",
    )
    
    for r in results:
        if r.ok:
            print(r.host, "downloaded log to:", r.dst)
        else:
            log.error("Download failed", {"host": r.host, "error": r.src})
```

### Preventing Naming Collisions

When downloading a file from multiple hosts to the same local destination directory, Starkite automatically appends the hostname as a suffix to the local file path (e.g., `./logs/error.log.alice-node-1.local`). This ensures that downloads from different nodes do not overwrite each other.

