#!/usr/bin/env kite
# known_hosts_rotation.star - Inspect, append, and rotate SSH known_hosts entries
#
# Usage:
#   kite run ./examples/core/ssh/known_hosts_rotation.star
#   kite run --dry-run ./examples/core/ssh/known_hosts_rotation.star

def main():
    print("=" * 60)
    print("SSH known_hosts Maintenance & Host Key Rotation")
    print("=" * 60)

    # Use an isolated temporary known_hosts file for demonstration
    kh_file = (fs.path(temp_dir()) / ("kh_" + str(time.now().unix_nano))).string
    target_node = "edge-node-1.local"

    # Generate an ephemeral keypair for demonstration
    kp1 = ssh.keygen(type="ed25519", comment="initial-key")
    kp2 = ssh.keygen(type="ed25519", comment="rotated-key")

    # 1. Programmatically append host keys (add_known_host)
    print("\n[1/4] Adding host key entries...")
    e1 = ssh.add_known_host(
        host    = target_node,
        key     = kp1.public_key,
        path    = kh_file,
        hash    = True,
        comment = "provisioned-v1",
    )
    printf("  Added entry: host=%s, type=%s, hashed=%v\n", e1.host, e1.type, e1.hashed)
    printf("  Fingerprint: %s\n", e1.fingerprint)

    # Add a plaintext secondary node
    e2 = ssh.add_known_host(
        host    = "gateway.lan",
        key     = kp1.public_key,
        path    = kh_file,
        hash    = False,
        comment = "gateway-unhashed",
    )
    printf("  Added entry: host=%s, type=%s, hashed=%v\n", e2.host, e2.type, e2.hashed)

    # 2. Query known_hosts entries (find_known_hosts)
    print("\n[2/4] Querying host key records...")
    records = ssh.find_known_hosts(target_node, path=kh_file)
    printf("  Found %d record(s) matching %q:\n", len(records), target_node)
    for r in records:
        printf("    - Line %d: host=%s fp=%s hashed=%v comment=%q\n",
            r.line_number, r.host, r.fingerprint, r.hashed, r.comment)

    # Safe error handling with try_find_known_hosts
    safe_lookup = ssh.try_find_known_hosts(target_node, path=kh_file)
    if safe_lookup.ok:
        printf("  Safe lookup succeeded with %d result(s)\n", len(safe_lookup.value))

    # 3. Simulate Node Re-Provisioning / Host Key Rotation
    print("\n[3/4] Performing Host Key Rotation for re-imaged node...")
    printf("  Node %q was re-imaged with new host key (fp=%s)\n", target_node, kp2.fingerprint)

    # Prune stale entries (equivalent to ssh-keygen -R)
    removed_count = ssh.remove_known_host(target_node, path=kh_file)
    printf("  Pruned %d stale record(s) from %s\n", removed_count, kh_file)

    # Verify stale records are evicted
    remaining = ssh.find_known_hosts(target_node, path=kh_file)
    printf("  Verified remaining records for %q: %d\n", target_node, len(remaining))

    # 4. Append new rotated host key
    print("\n[4/4] Storing rotated host key...")
    new_entry = ssh.add_known_host(
        host    = target_node,
        key     = kp2.public_key,
        path    = kh_file,
        hash    = True,
        comment = "reprovisioned-v2",
    )
    printf("  Stored new host key (fp=%s)\n", new_entry.fingerprint)

    # Final audit
    final_records = ssh.find_known_hosts(target_node, path=kh_file)
    printf("  Final verification: %d active key (fp=%s)\n", len(final_records), final_records[0].fingerprint)
    print("\n" + "=" * 60)
    print("Host key rotation workflow completed successfully.")
    print("=" * 60)
