# workspace_audit.star - Repository Status & Health Inspection
#
# Adheres to Starkite Tier 1 Agent Invariants:
#   1. def main(): entrypoint without top-level invocation
#   2. Safe os.try_exec() with argv (prevents shell injection)
#   3. try_ Result checking before accessing properties
#   4. Starlark native string methods (s.split, s.startswith, s.strip)

def audit_git_status():
    res = os.try_exec("git", ["status", "--porcelain"])
    if not res.ok:
        log.error("Git status failed with exit code %d: %s" % (res.code, res.error))
        return

    stdout = res.stdout.strip()
    if not stdout:
        log.info("Working copy is completely clean")
        return

    lines = stdout.split("\n")
    modified = 0
    untracked = 0
    for line in lines:
        if line.startswith("??"):
            untracked += 1
        else:
            modified += 1

    log.warn("Working copy has uncommitted changes (%d modified, %d untracked):" % (modified, untracked))
    for line in lines[:8]:
        log.warn("  " + line)
    if len(lines) > 8:
        log.warn("  ... and %d more" % (len(lines) - 8))

def audit_directory_sizes():
    p = fs.path(".")
    entries = p.glob("*")
    log.info("Inspecting top-level entries in workspace (%d found)" % len(entries))
    dirs = 0
    files = 0
    for entry in entries:
        if entry.is_dir():
            dirs += 1
        else:
            files += 1
    log.info("Workspace composition: %d directories, %d files" % (dirs, files))

def main():
    log.info("Starting Starkite Workspace Health Audit...")
    audit_git_status()
    audit_directory_sizes()
    log.info("Workspace Health Audit completed successfully")
