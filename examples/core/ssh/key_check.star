#!/usr/bin/env kite
# key_check.star - Probe remote hosts for public key authorization without logging in
#
# Usage:
#   kite run ./key_check.star
#   HOST=192.168.1.50 USER=deploy kite run ./key_check.star

def main():
    target_host = env("HOST", "127.0.0.1")
    target_user = env("USER", "root")
    target_port = int(env("PORT", "22"))
    key_path = env("KEY", "~/.ssh/id_ed25519.pub")

    print("=" * 60)
    print("SSH Remote Key Acceptance Probe (ssh.key_check)")
    print("=" * 60)
    printf("Host: %s:%d | User: %s | Key: %s\n\n", target_host, target_port, target_user, key_path)

    # 1. Standalone in-protocol key check (requires NO private key, NO password, NO session)
    print("[1/3] Probing remote host for public key authorization...")
    res = ssh.try_key_check(
        key            = key_path,
        hosts          = [target_host],
        user           = target_user,
        port           = target_port,
        timeout        = "3s",
        host_key_check = False,
    )
    if not res.ok:
        print("  Key check failed:", res.error)
        return

    for item in res.value:
        if item.ok:
            if item.accepted:
                printf("  [%s] Status: ACCEPTED (public key already present in authorized_keys)\n", item.host)
            else:
                printf("  [%s] Status: NOT ACCEPTED (key not found in authorized_keys)\n", item.host)
            printf("  Key Type:    %s\n", item.key_type)
            printf("  Fingerprint: %s\n\n", item.fingerprint)
        else:
            printf("  [%s] Connection Error: %s\n\n", item.host, item.error)

    # 2. Client-level key check
    print("[2/3] Probing via configured SSHClient instance...")
    client = ssh.config(
        hosts          = [target_host],
        port           = target_port,
        auth           = {"user": target_user},
        host_key_check = False,
    )
    client_check = client.try_key_check(key_path)
    if client_check.ok:
        for item in client_check.value:
            printf("  [%s] Accepted via client: %s\n", item.host, item.accepted)

    # 3. Smart copy_id with key_check=True (default)
    print("\n[3/3] Running copy_id with key_check=True...")
    print("  (If key is already accepted, installation and password prompt are bypassed)")
    copy_results = client.try_copy_id(key=key_path, key_check=True)
    if copy_results.ok:
        for r in copy_results.value:
            printf("  [%s] Result: %s\n", r.host, r.stdout.strip())
    else:
        printf("  Copy ID failed: %s\n", copy_results.error)
