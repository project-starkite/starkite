# sandbox_strict_test.star — Integration tests for the strict sandbox
# profile. Driven by tests/sandbox/integration_test.go on Linux only.
#
# Each test asserts one property of the strict profile. Run via:
#   kite test ./tests/sandbox/sandbox_strict_test.star --sandbox=strict
#
# The strict profile is the offline counterpart to default:
#   - Network: gVisor netstack with no host bridging (loopback only).
#   - Filesystem: $CWD rw + /tmp tmpfs only. NO /etc/ssl/certs, NO
#     /etc/resolv.conf, NO /etc/hosts, NO /etc/nsswitch.conf.
#
# The Go driver runs this file from a clean temp directory (NOT under
# $HOME) so the credential-isolation tests aren't confounded by the
# user's home dir being the $CWD bind-mount target.

# --- positive: things that should work under strict profile ---

def test_hello_world():
    """Sandbox boots, basic Starlark + builtins run, exit 0."""
    msg = "hello-from-strict-sandbox"
    assert(len(msg) == 25, "string ops must work; got len=%d" % len(msg))
    assert(msg.upper() == "HELLO-FROM-STRICT-SANDBOX", "string upper must work")
    print(msg)

def test_cwd_writable():
    """$CWD bound writable; written files round-trip through the bind."""
    p = path("./test_cwd.out")
    p.write_text("written-from-strict")
    got = p.read_text()
    assert(got == "written-from-strict", "round-trip mismatch: %s" % got)
    p.remove()

def test_tmpfs_writable():
    """/tmp is a private writable tmpfs."""
    p = path("/tmp/sandbox_strict_tmpfs")
    p.write_text("scratch")
    got = p.read_text()
    assert(got == "scratch", "tmpfs round-trip mismatch: %s" % got)

def test_http_loopback():
    """In-sandbox http.server + http.url round-trip via netstack loopback.
    Strict has its own netstack (gVisor's, not host) but loopback works
    within that netstack — a server and client in the same script can talk."""
    def handler(req):
        return {"status": 200, "body": "strict-ok"}

    srv = http.server()
    srv.handle("/ping", handler)
    srv.start(port=0)
    port = srv.port()
    assert(port > 0, "http.server should bind a port")

    resp = http.url("http://127.0.0.1:%d/ping" % port).get()
    assert(resp.status_code == 200, "expected 200, got %d" % resp.status_code)
    assert(resp.get_text() == "strict-ok", "body mismatch: %s" % resp.get_text())

    srv.shutdown()

def test_ssh_loopback_password():
    """In-sandbox ssh.test_server + ssh.exec round-trip via password auth
    on loopback. Same isolation as http_loopback but for SSH."""
    srv = ssh.test_server(user="testuser", password="testpass")
    srv.handle_exec(lambda cmd: ("strict-hello\n", "", 0))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"],
        user="testuser",
        password="testpass",
        port=srv.port(),
        host_key_check=False,
        max_retries=0,
    )
    results = client.exec("echo strict-hello")
    assert(len(results) == 1, "should have 1 result")
    assert(results[0].ok == True, "ssh.exec should succeed: %s" % results[0].stderr)
    assert(results[0].stdout == "strict-hello\n", "stdout mismatch: %s" % results[0].stdout)
    srv.shutdown()

def test_kernel_isolation():
    """Proves we're inside gVisor (sentry kernel), not the host kernel."""
    if not exists("/proc/version"):
        fail("/proc/version not present — gVisor /proc mount missing")
    content = read_text("/proc/version").lower()
    print("proc/version: %s" % content.strip())
    is_gvisor = ("gvisor" in content) or ("sentry" in content)
    assert(is_gvisor,
        "expected gVisor sentry marker in /proc/version; got: %s" % content)

# --- negative: things that should be blocked / invisible under strict ---

def test_no_etc_ssl_certs():
    """Strict drops the curated /etc/ssl/certs mount — TLS roots invisible."""
    assert(not exists("/etc/ssl/certs"),
        "/etc/ssl/certs must NOT be mounted under strict (default mounts it; strict does not)")

def test_no_etc_resolv_conf():
    """Strict drops /etc/resolv.conf — no DNS resolver config."""
    assert(not exists("/etc/resolv.conf"),
        "/etc/resolv.conf must NOT be mounted under strict")

def test_etc_passwd_invisible():
    """Host /etc/passwd is NOT mounted (same as default)."""
    assert(not exists("/etc/passwd"),
        "/etc/passwd must not be visible inside strict sandbox")

def test_home_invisible():
    """Host /home and /root are NOT mounted (driver runs from non-$HOME cwd)."""
    assert(not exists("/home"), "/home must not be visible (driver runs from non-$HOME cwd)")
    assert(not exists("/root"), "/root must not be visible")

def test_no_outbound_network():
    """No host bridging: outbound to non-loopback addresses must fail.

    We use a non-loopback IP (10.255.255.1, RFC1918 likely-unreachable)
    with a short timeout. try_get returns a Result; .ok must be False.
    A successful connect would mean the strict profile leaked host
    network reachability."""
    result = http.url("http://10.255.255.1:1/").try_get(timeout="2s")
    assert(not result.ok,
        "outbound to non-loopback should fail under strict; got result.ok=True")
    print("outbound blocked as expected: %s" % result.error)
