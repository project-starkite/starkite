#!/usr/bin/env kite
# remote-check.starctl - Check health of remote servers via SSH
#
# Usage:
#   kite run remote-check.starctl
#   SSH_USER=admin SSH_KEY=~/.ssh/id_rsa kite run remote-check.starctl
#
# Environment variables:
#   HOSTS     - Comma-separated list of hosts (default: localhost)
#   SSH_USER  - SSH username (default: root)
#   SSH_KEY   - Path to SSH private key (default: ~/.ssh/id_rsa)

# =============================================================================
# HELPERS
# =============================================================================

def print_table(headers, rows):
    """Print a formatted table."""
    t = table.new(headers)
    for row in rows:
        t.add_row(*row)
    print(t.render())

# =============================================================================
# HEALTH CHECK FUNCTIONS
# =============================================================================

def check_connectivity(ssh_client):
    """Test SSH connectivity to all hosts."""
    print("[1/5] Testing connectivity...")
    results = ssh_client.exec("echo 'OK'")

    status = []
    failed = []
    for r in results:
        if r.stderr:
            status.append([r.host, "FAILED", r.stderr[:50]])
            failed.append(r.host)
        else:
            status.append([r.host, "OK", ""])

    print_table(["HOST", "STATUS", "ERROR"], status)
    print("")

    return failed

def check_uptime(ssh_client):
    """Get uptime from all hosts."""
    print("[2/5] Checking uptime...")
    results = ssh_client.exec("uptime -p 2>/dev/null || uptime")

    rows = []
    for r in results:
        if r.ok:
            rows.append([r.host, r.stdout.strip()])
        else:
            rows.append([r.host, "N/A"])

    print_table(["HOST", "UPTIME"], rows)
    print("")

def check_load(ssh_client):
    """Get system load from all hosts."""
    print("[3/5] Checking system load...")
    results = ssh_client.exec("cat /proc/loadavg")

    rows = []
    for r in results:
        if r.ok:
            parts = r.stdout.split()
            if len(parts) >= 3:
                load1, load5, load15 = parts[0], parts[1], parts[2]
                rows.append([r.host, load1, load5, load15])
            else:
                rows.append([r.host, "N/A", "N/A", "N/A"])
        else:
            rows.append([r.host, "N/A", "N/A", "N/A"])

    print_table(["HOST", "1 MIN", "5 MIN", "15 MIN"], rows)
    print("")

def check_memory(ssh_client):
    """Get memory usage from all hosts."""
    print("[4/5] Checking memory usage...")
    results = ssh_client.exec("free -h | grep Mem")

    rows = []
    for r in results:
        if r.ok:
            parts = r.stdout.split()
            if len(parts) >= 4:
                total, used, free = parts[1], parts[2], parts[3]
                rows.append([r.host, total, used, free])
            else:
                rows.append([r.host, "N/A", "N/A", "N/A"])
        else:
            rows.append([r.host, "N/A", "N/A", "N/A"])

    print_table(["HOST", "TOTAL", "USED", "FREE"], rows)
    print("")

def check_disk(ssh_client):
    """Get disk usage from all hosts."""
    print("[5/5] Checking disk usage...")
    results = ssh_client.exec("df -h / | tail -1")

    rows = []
    for r in results:
        if r.ok:
            parts = r.stdout.split()
            if len(parts) >= 5:
                size, used, avail, pct = parts[1], parts[2], parts[3], parts[4]
                rows.append([r.host, size, used, avail, pct])
            else:
                rows.append([r.host, "N/A", "N/A", "N/A", "N/A"])
        else:
            rows.append([r.host, "N/A", "N/A", "N/A", "N/A"])

    print_table(["HOST", "SIZE", "USED", "AVAILABLE", "USE%"], rows)
    print("")

# =============================================================================
# MAIN
# =============================================================================

def main():
    # Parse hosts from environment
    hosts_str = env("HOSTS", "localhost")
    HOSTS = hosts_str.split(",")

    # Trim whitespace from host names
    HOSTS = [h.strip() for h in HOSTS]

    SSH_USER = env("SSH_USER", "root")
    SSH_KEY = env("SSH_KEY", "~/.ssh/id_rsa")

    printf("Checking %d hosts: %s\n", len(HOSTS), ", ".join(HOSTS))
    printf("SSH User: %s\n", SSH_USER)
    printf("SSH Key: %s\n", SSH_KEY)
    print("")

    # Configure SSH provider
    ssh_client = ssh.config(
        user = SSH_USER,
        key = SSH_KEY,
        hosts = HOSTS,
        timeout = "10s",
        max_retries = 2,
        exec_policy = "concurrent",
    )

    print("=" * 60)
    print("Remote Server Health Check")
    printf("Generated: %s\n", time.format(time.now(), time.RFC3339))
    print("=" * 60)
    print("")

    failed_hosts = check_connectivity(ssh_client)

    if failed_hosts:
        printf("Warning: %d hosts unreachable\n", len(failed_hosts))
        print("Continuing with reachable hosts...")
        print("")

        # Filter to only reachable hosts
        reachable = [h for h in HOSTS if h not in failed_hosts]
        if not reachable:
            fail("No hosts reachable")

        # Reconfigure SSH client with only reachable hosts
        ssh_client = ssh.config(
            user = SSH_USER,
            key = SSH_KEY,
            hosts = reachable,
            timeout = "10s",
            exec_policy = "concurrent",
        )

    check_uptime(ssh_client)
    check_load(ssh_client)
    check_memory(ssh_client)
    check_disk(ssh_client)

    print("=" * 60)
    print("Health check complete!")
    print("=" * 60)

# Run main
main()
