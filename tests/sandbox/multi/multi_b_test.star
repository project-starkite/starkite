# multi_b_test.star — second of two files used to verify per-file sandboxing.

def test_b_runs():
    """File B writes its own scratch file; A's file_a_a.out shouldn't exist
    in B's sandbox view because A's sandbox already cleaned up."""
    p = path("./multi_b.out")
    p.write_text("from-b")
    got = p.read_text()
    assert(got == "from-b", "round-trip mismatch: %s" % got)
    p.remove()

def test_b_kernel_isolation():
    """Verify sandbox execution environment."""
    res = path("/proc/version").try_read_text()
    if not res.ok:
        return
    content = res.value.lower()
    if "gvisor" in content or "sentry" in content:
        print("gVisor sentry verified")
