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

def test_a_kernel_is_gvisor():
    """Each per-file sandbox is real — /proc/version reports gVisor."""
    content = read_text("/proc/version").lower()
    assert("gvisor" in content or "sentry" in content,
        "per-file sandbox should be gVisor; got: %s" % content)
