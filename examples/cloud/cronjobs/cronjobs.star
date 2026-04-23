#!/usr/bin/env kite
# cronjobs.star - Generate CronJobs from a simple config list
#
# Defines a batch of CronJobs as a flat list and renders them as a
# multi-document YAML stream via yaml.encode(). Easy to extend —
# just add another entry to the jobs list.
#
# Usage:
#   kite run examples/cloud/cronjobs/cronjobs.star | kubectl apply -f -
#   kite run examples/cloud/cronjobs/cronjobs.star --var ns=batch | kubectl diff -f -
#   kite run examples/cloud/cronjobs/cronjobs.star --var ns=production --var image=myapp:v3 > k8s/cronjobs.yaml

# Job definitions — add/remove entries as needed
jobs = [
    {"name": "cleanup-tmp",    "schedule": "0 */6 * * *",  "cmd": "find /tmp -mtime +1 -delete"},
    {"name": "sync-reports",   "schedule": "30 2 * * *",   "cmd": "python /app/sync_reports.py"},
    {"name": "db-vacuum",      "schedule": "0 3 * * 0",    "cmd": "psql $DB_URL -c 'VACUUM ANALYZE'"},
    {"name": "send-digest",    "schedule": "0 8 * * 1-5",  "cmd": "python /app/send_digest.py"},
    {"name": "rotate-logs",    "schedule": "0 0 * * *",    "cmd": "/app/rotate-logs.sh"},
]

def main():
    ns = var_str("ns", "default")
    image = var_str("image", "myapp:latest")

    # --- Generate CronJob manifests ----------------------------------------
    resources = []
    for job in jobs:
        cj = k8s.obj.cron_job(
            name=job["name"],
            namespace=ns,
            schedule=job["schedule"],
            concurrency_policy="Forbid",
            history_limit=3,
            failed_history=1,
            containers=[
                k8s.obj.container(
                    name=job["name"],
                    image=image,
                    command=["/bin/sh", "-c", job["cmd"]],
                    resources=k8s.obj.resource_requirements(
                        requests={"cpu": "50m", "memory": "64Mi"},
                        limits={"cpu": "200m", "memory": "256Mi"},
                    ),
                ),
            ],
            restart_policy="Never",
        )
        resources.append(cj)

    print(k8s.yaml(resources))

main()
