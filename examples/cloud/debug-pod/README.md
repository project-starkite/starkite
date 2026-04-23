# Debug Pod

Interactive pod debugging toolkit — gathers diagnostic info from a pod in one
shot. Useful as a first response when something is misbehaving.

## What it demonstrates

- **Tier 1** pod operations: `k.describe()`, `k.logs()`, `k.exec()`, `k.port_forward()`
- Pod condition and container status inspection
- Event log review
- In-container diagnostic commands
- Optional port-forward with HTTP health check

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `pod.name` | **Yes** | — | Name of the pod to debug |
| `namespace` | No | `default` | Pod's namespace |
| `container` | No | (first container) | Specific container to target for logs and exec |
| `tail.lines` | No | `50` | Number of log lines to show |
| `port` | No | `0` (skip) | Remote port for port-forwarding; set to `0` to skip |

## Usage

```bash
# Basic pod debug
kite run examples/cloud/debug-pod/debug-pod.star --var pod.name=nginx-abc123

# Target a specific namespace and container
kite run examples/cloud/debug-pod/debug-pod.star --var pod.name=nginx-abc123 --var namespace=production --var container=sidecar

# With port-forward
kite run examples/cloud/debug-pod/debug-pod.star --var pod.name=nginx-abc123 --var port=8080
```

## What it creates

Nothing — this is a read-only debugging script (port-forward is temporary).

## How it works

1. **Pod info** (`k.describe()`): shows phase, node, IP, start time
2. **Conditions**: lists pod conditions with pass/fail markers
3. **Container statuses**: shows state, readiness, restart count, and waiting reasons
4. **Events**: prints the last 10 events for the pod
5. **Logs**: shows recent log lines, falls back to previous container logs if current are empty
6. **Exec diagnostics**: runs `id`, `cat /etc/os-release`, `df -h /`, and `printenv` inside the container
7. **Port-forward** (optional): forwards a local port to the pod and performs an HTTP health check
