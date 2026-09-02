#!/usr/bin/env kite
# keygen_and_bootstrap.star - Generate SSH keys and bootstrap fleet public keys
#
# Usage:
#   kite run ./keygen_and_bootstrap.star

def main():
    print("=" * 60)
    print("SSH Keypair Generation & Fleet Bootstrap")
    print("=" * 60)

    # 1. Generate new Ed25519 cluster management keypair
    print("[1/3] Generating Ed25519 keypair...")
    key_path = (fs.path(temp_dir()) / "id_cluster_ed25519").string
    keys = ssh.keygen(
        type       = "ed25519",
        comment    = "cluster-admin-2026",
        path       = key_path,
        overwrite  = True,
    )
    printf("  Key Type:    %s\n", keys.type)
    printf("  Fingerprint: %s\n", keys.fingerprint)
    printf("  Private Key: %s\n", keys.path)
    printf("  Public Key:  %s\n", keys.pub_path)
    printf("  Public Key Content: %s\n", keys.public_key.strip())

    # 2. Distribute public key to fleet hosts
    target_nodes = ["192.168.1.101", "192.168.1.102", "192.168.1.103"]
    print("\n[2/3] Distributing public key to cluster nodes...")
    
    copy_results = ssh.copy_id(
        key          = keys.public_key,
        hosts        = target_nodes,
        user         = "admin",
        as_user      = "deploy",   # Install into ~deploy/.ssh/authorized_keys
        sudo         = True,
        dry_run      = True,       # Set to False for live deployment
    )
    for r in copy_results:
        printf("  [%s] %s\n", r.host, r.stdout.strip())

    # 3. Verify passwordless access with the newly created private key
    print("\n[3/3] Verifying access with new key...")
    verify_client = ssh.config(
        hosts   = target_nodes,
        auth    = {
            "user": "deploy",
            "key":  keys.path,
        },
        dry_run = True,
    )
    verify_results = verify_client.exec("hostname")
    for r in verify_results:
        printf("  [%s] Verified ok=%s\n", r.host, str(r.ok))

    # Clean up generated key files
    fs.path(keys.path).remove()
    fs.path(keys.pub_path).remove()
