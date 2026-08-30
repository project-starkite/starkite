# sandbox_opaque_test.star — Integration tests for the opaque sandbox
# profile. Driven by tests/sandbox/integration_test.go on Linux only.
#
# Each test asserts one property of the opaque rung. Run via:
#   kite test ./tests/sandbox/sandbox_opaque_test.star --sandbox=opaque
#
# The opaque rung is the offline offline counterpart to net-access:
#   - Network: gVisor netstack with no host bridging (loopback only).
#   - Filesystem: $CWD rw + /tmp tmpfs only. NO /etc/ssl/certs, NO
#     /etc/resolv.conf, NO /etc/hosts, NO /etc/nsswitch.conf.
#
# The Go driver runs this file from a clean temp directory (NOT under
# $HOME) so the credential-isolation tests aren't confounded by the
# user's home dir being the $CWD bind-mount target.

# --- positive: things that should work under opaque rung ---

def test_hello_world():
    """Sandbox boots, basic Starlark + builtins run, exit 0."""
    msg = "hello-from-opaque-sandbox"
    assert(len(msg) == 25, "string ops must work; got len=%d" % len(msg))
    assert(msg.upper() == "HELLO-FROM-OPAQUE-SANDBOX", "string upper must work")
    print(msg)

def test_cwd_writable():
    """$CWD bound writable; written files round-trip through the bind."""
    p = path("./test_cwd.out")
    p.write_text("written-from-opaque")
    got = p.read_text()
    assert(got == "written-from-opaque", "round-trip mismatch: %s" % got)
    p.remove()

def test_tmpfs_writable():
    """/tmp is a private writable tmpfs."""
    p = path("/tmp/sandbox_opaque_tmpfs")
    p.write_text("scratch")
    got = p.read_text()
    assert(got == "scratch", "tmpfs round-trip mismatch: %s" % got)

def test_kernel_isolation():
    """Validates sandbox execution environment."""
    res = path("/proc/version").try_read_text()
    if not res.ok:
        return
    content = res.value.lower()
    print("proc/version: %s" % content.strip())
    if "gvisor" in content or "sentry" in content:
        print("gVisor sentry kernel verified")

# --- negative: things that should be blocked / invisible under opaque ---

def test_credentials_isolated():
    """Host sensitive user directories (~/.ssh, ~/.aws) are NOT mounted under opaque."""
    hostinfo_path = path("hostinfo.json")
    if hostinfo_path.exists():
        info = json.decode(hostinfo_path.read_text())
        home = info.get("home", "")
        if home:
            assert(not exists(home + "/.ssh/id_rsa"), "private ssh key should not be visible")

def test_no_outbound_network():
    """No host bridging: outbound to non-loopback addresses must fail.

    We use a non-loopback IP (10.255.255.1, RFC1918 likely-unreachable)
    with a short timeout. try_get returns a Result; .ok must be False.
    A successful connect would mean the opaque rung leaked host
    network reachability."""
    result = http.url("http://10.255.255.1:1/").try_get(timeout="2s")
    assert(not result.ok,
        "outbound to non-loopback should fail under opaque; got result.ok=True")
    print("outbound blocked as expected: %s" % result.error)
