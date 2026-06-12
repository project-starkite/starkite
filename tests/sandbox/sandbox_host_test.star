# sandbox_host_test.star — Integration tests for the host sandbox rung.
#
# Each test asserts one property of the host rung. Run via:
#   kite test ./tests/sandbox/sandbox_host_test.star --sandbox=host
#
# The host rung is net-access plus read-only $HOME, /usr, /bin, /lib:
# read the host, write only the tree. The driver stages hostinfo.json
# (containing the host $HOME path) into $CWD before the run.

hostinfo = json.decode(read_text("hostinfo.json"))
HOME = hostinfo["home"]

# --- positive: host-read capabilities ---

def test_home_visible():
    assert(exists(HOME), "$HOME (%s) must be visible inside the host rung" % HOME)

def test_usr_visible():
    assert(exists("/usr"), "/usr must be visible inside the host rung")

def test_host_binary_runs():
    out = exec("/usr/bin/env echo host-tool-ok")
    assert("host-tool-ok" in out, "host binary should execute; got: %s" % out)

def test_cwd_writable():
    p = path("host_rung_out.txt")
    p.write_text("written-from-host-rung")
    assert(p.read_text() == "written-from-host-rung", "$CWD round-trip failed")

# --- negative: host-write and beyond-rung surfaces ---

def test_home_not_writable():
    # Probe via the mounted host shell: the inner write fails on the ro
    # mount while the wrapper always exits 0, so exec() does not raise.
    out = exec("sh -c 'echo x > %s/sandbox_host_probe.txt 2>/dev/null && echo WROTE || echo DENIED'" % HOME)
    assert("DENIED" in out, "$HOME must be read-only inside the host rung; got: %s" % out)

def test_usr_not_writable():
    out = exec("sh -c 'echo x > /usr/sandbox_host_probe.txt 2>/dev/null && echo WROTE || echo DENIED'")
    assert("DENIED" in out, "/usr must be read-only inside the host rung; got: %s" % out)

def test_etc_passwd_invisible():
    assert(not exists("/etc/passwd"), "/etc/passwd is not part of the host rung mount set")

def test_gvisor_marker():
    if not exists("/proc/version"):
        return
    content = read_text("/proc/version")
    assert("gvisor" in content.lower(), "expected gVisor sentry marker in /proc/version; got: %s" % content)
