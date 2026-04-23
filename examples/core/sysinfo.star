#!/usr/bin/env kite
# sysinfo.star - Collect and display system information

def main():
    print("=" * 60)
    print("System Information Report")
    print("=" * 60)

    # Basic system info
    printf("\n[Host Information]\n")
    printf("  Hostname:  %s\n", hostname())
    printf("  User:      %s (uid=%d, gid=%d)\n", username(), userid(), groupid())
    printf("  Directory: %s\n", cwd())

    # Operating system info
    os_info = os.exec("uname -a")
    printf("\n[Operating System]\n")
    printf("  %s\n", os_info.strip())

    # CPU information
    cpu_result = os.try_exec("cat /proc/cpuinfo | grep 'model name' | head -1 | cut -d':' -f2")
    if cpu_result.ok and cpu_result.stdout:
        printf("\n[CPU]\n")
        printf("  Model: %s\n", cpu_result.stdout.strip())

    # Memory information
    mem_result = os.try_exec("free -h | grep Mem")
    if mem_result.ok and mem_result.stdout:
        fields = mem_result.stdout.split()
        if len(fields) >= 3:
            printf("\n[Memory]\n")
            printf("  Total:     %s\n", fields[1])
            printf("  Used:      %s\n", fields[2])
            if len(fields) >= 4:
                printf("  Free:      %s\n", fields[3])

    # Disk usage
    disk_info = os.exec("df -h / | tail -1")
    if disk_info:
        fields = disk_info.split()
        if len(fields) >= 5:
            printf("\n[Disk Usage (/)]\n")
            printf("  Size:      %s\n", fields[1])
            printf("  Used:      %s (%s)\n", fields[2], fields[4])
            printf("  Available: %s\n", fields[3])

    # Network interfaces
    net_result = os.try_exec("ip -brief addr 2>/dev/null || ifconfig 2>/dev/null | head -20")
    if net_result.ok and net_result.stdout:
        printf("\n[Network Interfaces]\n")
        for line in net_result.stdout.strip().split("\n")[:5]:
            printf("  %s\n", line)

    # Processes
    proc_result = os.try_exec("ps aux --sort=-%mem | head -6")
    if proc_result.ok and proc_result.stdout:
        printf("\n[Top Processes by Memory]\n")
        lines = proc_result.stdout.strip().split("\n")
        for line in lines:
            printf("  %s\n", line[:80])

    print("\n" + "=" * 60)
    printf("Report generated at: %s\n", time.format(time.now(), time.RFC3339))
    print("=" * 60)

# Run main
main()
