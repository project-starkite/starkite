# CronJobs

Generate a batch of CronJobs from a flat config list — easy to extend by just
adding another entry.

## What it demonstrates

- **Tier 1** (raw YAML generation): `yaml.encode()` for each CronJob manifest
- Data-driven manifest generation from a simple list
- CronJob best practices: `concurrencyPolicy: Forbid`, history limits, resource constraints

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ns` | No | `default` | Target namespace for all CronJobs |
| `image` | No | `myapp:latest` | Container image for all jobs |

## Usage

```bash
# Generate and apply
kite run examples/cloud/cronjobs/cronjobs.star | kubectl apply -f -

# Target a specific namespace
kite run examples/cloud/cronjobs/cronjobs.star --var ns=batch | kubectl diff -f -

# Custom image, save to file
kite run examples/cloud/cronjobs/cronjobs.star --var ns=production --var image=myapp:v3 > k8s/cronjobs.yaml
```

## What it creates

5 CronJobs:

| Name | Schedule | Command |
|------|----------|---------|
| `cleanup-tmp` | Every 6 hours | `find /tmp -mtime +1 -delete` |
| `sync-reports` | Daily at 02:30 | `python /app/sync_reports.py` |
| `db-vacuum` | Weekly on Sunday at 03:00 | `psql $DB_URL -c 'VACUUM ANALYZE'` |
| `send-digest` | Weekdays at 08:00 | `python /app/send_digest.py` |
| `rotate-logs` | Daily at midnight | `/app/rotate-logs.sh` |

Each CronJob has:
- `concurrencyPolicy: Forbid` — prevents overlapping runs
- Resource requests (50m CPU, 64Mi) and limits (200m CPU, 256Mi)
- History limits: 3 successful, 1 failed

## How it works

1. Defines a `jobs` list with name, schedule, and command for each CronJob
2. Iterates the list, building a full CronJob manifest dict for each entry
3. Encodes each with `yaml.encode()` and prints as a multi-document YAML stream
