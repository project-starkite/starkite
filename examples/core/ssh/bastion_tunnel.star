#!/usr/bin/env kite
# bastion_tunnel.star - Connect to private hosts through a bastion jump host
#
# Usage:
#   kite run ./bastion_tunnel.star

def main():
    print("=" * 60)
    print("Bastion Jump Host Tunneling Example")
    print("=" * 60)

    # Private internal IPs behind the bastion
    internal_hosts = ["10.0.10.5", "10.0.10.6", "10.0.10.7"]
    bastion_host = env("BASTION_HOST", "bastion.example.com")
    bastion_user = env("BASTION_USER", "bastion-user")

    printf("Bastion Gateway: %s@%s\n", bastion_user, bastion_host)
    printf("Target Hosts:    %s\n\n", ", ".join(internal_hosts))

    # Configure client to proxy through the bastion
    client = ssh.config(
        hosts         = internal_hosts,
        user          = "appuser",
        key           = "~/.ssh/id_ed25519",
        jump_host     = bastion_host,
        jump_user     = bastion_user,
        jump_key      = "~/.ssh/bastion_key",
        jump_port     = 22,
        dry_run       = True,  # Set to False for live execution
    )

    print("Running service check on private backend nodes through bastion tunnel...")
    results = client.exec("systemctl is-active backend-worker")
    
    t = table.new(["PRIVATE IP", "TUNNEL", "STATUS", "OUTPUT"])
    for r in results:
        status = "ACTIVE" if r.ok else "INACTIVE"
        t.add_row(r.host, bastion_host, status, r.stdout.strip())
    print(t.render())
