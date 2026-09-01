#!/usr/bin/env kite
# hello_ssh.star - Basic SSH connection and execution example
#
# Usage:
#   kite run ./hello_ssh.star
#   HOST=192.168.1.50 USER=admin kite run ./hello_ssh.star

def main():
    target_host = env("HOST", "127.0.0.1")
    ssh_user = env("USER", "root")
    ssh_key = env("KEY", "~/.ssh/id_ed25519")

    print("=" * 60)
    print("SSH Quickstart Example")
    print("=" * 60)
    printf("Target Host: %s\n", target_host)
    printf("User:        %s\n", ssh_user)
    printf("Key:         %s\n\n", ssh_key)

    # 1. Config-based client execution
    client = ssh.config(
        hosts    = target_host,
        user     = ssh_user,
        key      = ssh_key,
        dry_run  = True,  # Set to False to run against live hosts
    )

    print("[1/2] Executing uptime via client...")
    results = client.exec("uptime")
    for r in results:
        printf("  [%s] exit_code=%d ok=%s\n", r.host, r.code, str(r.ok))
        printf("  stdout: %s\n", r.stdout.strip())

    # 2. One-shot execution
    print("\n[2/2] Executing uname via one-shot ssh.exec...")
    results = ssh.exec(
        "uname -a",
        hosts   = target_host,
        user    = ssh_user,
        key     = ssh_key,
        dry_run = True,
    )
    for r in results:
        printf("  [%s] stdout: %s\n", r.host, r.stdout.strip())
