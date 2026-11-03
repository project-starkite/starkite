# demo_lsp.star - Starkite Language Server Demonstration Script
#
# Open this file in an LSP-enabled editor (VS Code, Neovim, Helix, Zed)
# to observe live autocompletion, hover docstrings, and diagnostics.

def audit_service(endpoint):
    # 1. Autocompletion Test: Type "http." below to inspect available methods
    res = http.try_get("https://" + endpoint + "/healthz", timeout="5s")

    # 2. Field Completion Test: Type "res." to see fields (ok, value, error, code)
    if res.ok:
        log.info("Endpoint %s is healthy (status=%d)" % (endpoint, res.value.status_code))
    else:
        log.warn("Health check failed for %s: %s" % (endpoint, res.error))

def process_configuration(path_str):
    # 3. Hover Documentation Test: Hover over "fs.path" or "fs.try_read_text"
    p = fs.path(path_str)
    if not p.exists():
        log.error("Configuration file missing: %s" % p.clean())
        return

    content_res = fs.try_read_text(p.clean())
    if content_res.ok:
        # 4. JSON Member Completion Test: Type "json."
        cfg = json.decode(content_res.value)
        log.info("Loaded configuration for service: %s" % cfg.get("service", "unknown"))

def main():
    log.info("Starkite LSP Demo executing under kernel sandbox")
    audit_service("httpbin.org")

# ==============================================================================
# 5. Diagnostic Linter Demonstrations (Uncomment to test editor warnings)
# ==============================================================================

# L001 Demonstration: Top-level entrypoint calls trigger warning L001
# main()

# L002 Demonstration: Accessing res.value without checking res.ok triggers L002
# raw = fs.try_read_text("test.txt")
# print(raw.value)
