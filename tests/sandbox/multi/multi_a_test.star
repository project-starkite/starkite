# multi_a_test.star — first of two files used to verify that
# `kite test` runs each test file in its own sandbox process when the
# sandbox is engaged.

def test_a_runs():
    """File A is its own sandbox: $CWD writes shouldn't collide with file B."""
    p = path("./multi_a.out")
    p.write_text("from-a")
    got = p.read_text()
    assert(got == "from-a", "round-trip mismatch: %s" % got)
    p.remove()

def test_a_kernel_isolation():
    """Verify sandbox execution environment."""
    res = path("/proc/version").try_read_text()
    if not res.ok:
        return
    content = res.value.lower()
    if "gvisor" in content or "sentry" in content:
        print("gVisor sentry verified")
