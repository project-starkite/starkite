#!/usr/bin/env kite
# fleet_exec.star - Concurrently execute commands and pipelines across a fleet
#
# Usage:
#   kite run ./fleet_exec.star

def main():
    print("=" * 60)
    print("Fleet Execution & Multi-Command Pipelines")
    print("=" * 60)

    # Define fleet of target hosts
    nodes = fleet.new([
        {"name": "web-01", "address": "192.168.1.10", "role": "frontend"},
        {"name": "web-02", "address": "192.168.1.11", "role": "frontend"},
        {"name": "app-01", "address": "192.168.1.20", "role": "backend"},
        {"name": "db-01",  "address": "192.168.1.30", "role": "database"},
    ])
    printf("Configured fleet with %d nodes.\n\n", nodes.count)

    # Configure client with worker pool concurrency limit
    client = ssh.config(
        fleet            = nodes,
        auth             = {
            "user": "deploy",
            "key":  "~/.ssh/id_ed25519",
        },
        exec_max_workers = 2,     # Limit concurrent active connections to 2
        dry_run          = True,  # Set to False for live execution
    )

    # 1. Single command across fleet
    print("[1/2] Checking system load across fleet...")
    uptime_results = client.exec("uptime")
    
    t = table.new(["HOST", "STATUS", "OUTPUT"])
    for r in uptime_results:
        status = "OK" if r.ok else "FAILED"
        t.add_row(r.host, status, r.stdout.strip() if r.ok else r.stderr.strip())
    print(t.render())
    print("")

    # 2. Multi-command batch pipeline with fail-fast policy
    print("[2/2] Running deployment pipeline across frontend nodes...")
    frontend_fleet = nodes.filter(role="frontend")
    
    frontend_client = ssh.config(
        fleet         = frontend_fleet,
        auth          = {
            "user": "deploy",
            "key":  "~/.ssh/id_ed25519",
        },
        dry_run       = True,
    )

    batch_results = frontend_client.exec(
        commands = [
            "git -C /var/www/app pull origin main",
            "systemctl restart webapp",
            "systemctl status webapp --no-pager",
        ],
        exec_on_error = "stop",  # Stop pipeline on any host if a step fails
        sudo          = True,
    )

    for br in batch_results:
        printf("Host %s: overall_ok=%s stopped_early=%s\n", br.host, str(br.ok), str(br.stopped_early))
        for i, step in enumerate(br.steps):
            printf("  Step %d: cmd='%s' code=%d\n", i + 1, step.cmd, step.code)
