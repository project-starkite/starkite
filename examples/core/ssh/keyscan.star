#!/usr/bin/env kite
# keyscan.star - Discover remote host keys and bootstrap known_hosts
#
# Usage:
#   kite run ./keyscan.star
#   HOST=192.168.1.50 kite run ./keyscan.star

def main():
    target_host = env("HOST", "127.0.0.1")
    scan_port = int(env("PORT", "22"))

    print("=" * 60)
    print("SSH Host Key Discovery (ssh.keyscan)")
    print("=" * 60)
    printf("Target Host: %s:%d\n\n", target_host, scan_port)

    # 1. In-memory host key discovery
    print("[1/4] Scanning remote host key in-memory...")
    res = ssh.try_keyscan(
        hosts   = [target_host],
        port    = scan_port,
        timeout = "3s",
    )
    if not res.ok:
        print("  Scan failed (host offline or port closed):", res.error)
        return

    keys = res.value
    for k in keys:
        printf("  Host:        %s:%d\n", k.host, k.port)
        printf("  Key Type:    %s\n", k.type)
        printf("  Fingerprint: %s\n", k.fingerprint)
        printf("  Public Key:  %s\n", k.public_key[:60] + "...")
        printf("  KnownHosts:  %s\n\n", k.line)

    # 2. Scanning specific key algorithms
    print("[2/4] Scanning with explicit algorithm filter (ed25519)...")
    ed_keys = ssh.try_keyscan(
        hosts = [target_host],
        port  = scan_port,
        type  = "ed25519",
    )
    if ed_keys.ok:
        for k in ed_keys.value:
            printf("  [%s] algorithm=%s fp=%s\n", k.host, k.type, k.fingerprint)

    # 3. Discover and save to custom known_hosts file
    print("\n[3/4] Scanning and persisting to known_hosts...")
    kh_file = (fs.path(temp_dir()) / "discovered_known_hosts").string
    ssh.keyscan(
        hosts = [target_host],
        port  = scan_port,
        save  = True,
        path  = kh_file,
    )
    printf("  Successfully saved host key to %s\n", kh_file)
    printf("  File content: %s\n", fs.read_text(kh_file).strip())

    # 4. Scanning through a client instance
    print("[4/4] Scanning via configured SSH client...")
    client = ssh.config(
        hosts = [target_host],
        port  = scan_port,
        auth  = {"user": env("USER", "root")},
    )
    client_keys = client.try_keyscan()
    if client_keys.ok:
        printf("  Discovered %d keys via client\n", len(client_keys.value))

    # Clean up temporary known_hosts file
    fs.path(kh_file).remove()
