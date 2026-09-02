#!/usr/bin/env kite
# file_sync.star - Upload configs and download remote logs across fleet nodes
#
# Usage:
#   kite run ./file_sync.star

def main():
    print("=" * 60)
    print("Fleet File Synchronization & Log Collection")
    print("=" * 60)

    target_nodes = ["web-01.lan", "web-02.lan"]
    
    client = ssh.config(
        hosts   = target_nodes,
        auth    = {
            "user": "deploy",
            "key":  "~/.ssh/id_ed25519",
        },
        dry_run = True,  # Set to False for live execution
    )

    # 1. Prepare and upload a local configuration file
    local_config = (fs.path(temp_dir()) / "app.env").string
    write_text(local_config, "ENV=production\nLOG_LEVEL=info\nPORT=8080\n")
    printf("Created local config: %s\n\n", local_config)

    print("[1/2] Uploading configuration to fleet hosts...")
    upload_results = client.upload(
        src  = local_config,
        dst  = "/etc/myapp/app.env",
        mode = "0644",
    )
    for res in upload_results:
        status = "SUCCESS" if res.ok else "FAILED"
        printf("  [%s] %s -> %s (%s, %d bytes)\n", res.host, res.src, res.dst, status, res.bytes)

    # 2. Download remote logs from all hosts
    local_log_dir = (fs.path(temp_dir()) / "logs_collected").string
    fs.path(local_log_dir).mkdir(parents=True)
    printf("\n[2/2] Collecting remote logs to %s...\n", local_log_dir)

    download_results = client.download(
        src = "/var/log/myapp.log",
        dst = local_log_dir + "/myapp.log",
    )
    for res in download_results:
        status = "SUCCESS" if res.ok else "FAILED"
        printf("  [%s] %s -> %s (%s, %d bytes)\n", res.host, res.src, res.dst, status, res.bytes)

    # Clean up local temporary files
    fs.path(local_config).remove()
    fs.path(local_log_dir).remove()
