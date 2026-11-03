# cloud_health_check.star - Concurrent Cloud Health Triage
#
# Adheres to Starkite Tier 1 Agent Invariants:
#   1. Concurrent worker pool fan-out via concur.map()
#   2. Safe http.try_get() with timeouts
#   3. Error handling on try_ Result before reading response status

def check_target(target):
    url = "https://" + target
    res = http.try_get(url, timeout="3s")
    if res.ok:
        return {
            "target": target,
            "alive": True,
            "status": res.value.status_code,
            "error": ""
        }
    return {
        "target": target,
        "alive": False,
        "status": 0,
        "error": res.error
    }

def main():
    log.info("Starting concurrent cloud health check...")
    endpoints = [
        "httpbin.org/status/200",
        "httpbin.org/status/204",
        "httpbin.org/status/404",
        "httpbin.org/delay/1",
    ]

    # Concurrently probe endpoints with worker concurrency limit of 2
    results = concur.map(endpoints, check_target, workers=2)

    passed = 0
    failed = 0
    for r in results:
        if r["alive"] and r["status"] < 400:
            passed += 1
            log.info("Endpoint %s OK (status=%d)" % (r["target"], r["status"]))
        else:
            failed += 1
            log.warn("Endpoint %s UNHEALTHY (status=%d, err=%s)" % (r["target"], r["status"], r["error"]))

    log.info("Health triage summary: %d passed, %d failed out of %d checked" % (passed, failed, len(results)))
