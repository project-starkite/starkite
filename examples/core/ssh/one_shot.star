#!/usr/bin/env kite
# one_shot.star - Demonstrate one-shot SSH execution without client initialization
#
# Usage:
#   kite run ./one_shot.star
#   HOST=192.168.1.50 USER=admin kite run ./one_shot.star

def main():
    target_hosts = [env("HOST", "127.0.0.1")]
    ssh_user = env("USER", "root")
    ssh_key = env("KEY", "~/.ssh/id_ed25519")

    print("=" * 60)
    print("One-Shot SSH Operations")
    print("=" * 60)
    printf("Target Hosts: %s\n", ", ".join(target_hosts))
    printf("User:         %s\n", ssh_user)
    printf("Key:          %s\n\n", ssh_key)

    # 1. Basic one-shot command execution (flat credentials)
    print("[1/5] Executing single command via ssh.exec...")
    results = ssh.exec(
        "uptime",
        hosts   = target_hosts,
        user    = ssh_user,
        key     = ssh_key,
        dry_run = True,  # Set to False for live execution
    )
    for r in results:
        printf("  [%s] exit_code=%d ok=%s\n", r.host, r.code, str(r.ok))
        printf("  stdout: %s\n", r.stdout.strip())

    # 2. Structured binary + arguments invocation
    print("\n[2/5] Executing command with structured arguments...")
    results = ssh.exec(
        "git",
        ["status", "--short"],
        hosts   = target_hosts,
        user    = ssh_user,
        key     = ssh_key,
        cwd     = "/var/www/app",
        dry_run = True,
    )
    for r in results:
        printf("  [%s] cmd: %s\n", r.host, r.cmd)

    # 3. Multi-command pipeline with fail-fast policy
    print("\n[3/5] Executing multi-command pipeline...")
    batch_results = ssh.exec(
        commands = [
            "systemctl is-active webapp",
            "systemctl reload webapp",
        ],
        hosts         = target_hosts,
        user          = ssh_user,
        key           = ssh_key,
        exec_on_error = "stop",
        sudo          = True,
        dry_run       = True,
    )
    for br in batch_results:
        printf("  [%s] overall_ok=%s stopped_early=%s\n", br.host, str(br.ok), str(br.stopped_early))
        for step in br.steps:
            printf("    step cmd='%s' code=%d\n", step.cmd, step.code)

    # 4. Safe one-shot execution with try_exec (non-throwing)
    print("\n[4/5] Safe execution with ssh.try_exec...")
    res = ssh.try_exec(
        "hostname",
        hosts   = target_hosts,
        user    = ssh_user,
        key     = ssh_key,
        dry_run = True,
    )
    if res.ok:
        for r in res.value:
            printf("  [%s] hostname: %s\n", r.host, r.stdout.strip())
    else:
        printf("  Execution failed gracefully: %s\n", res.error)

    # 5. One-shot public key installation via ssh.copy_id
    print("\n[5/5] Distributing public key via one-shot ssh.copy_id...")
    copy_results = ssh.copy_id(
        key      = "~/.ssh/id_ed25519.pub",
        hosts    = target_hosts,
        user     = ssh_user,
        as_user  = "deploy",
        sudo     = True,
        dry_run  = True,
    )
    for r in copy_results:
        printf("  [%s] %s\n", r.host, r.stdout.strip())
