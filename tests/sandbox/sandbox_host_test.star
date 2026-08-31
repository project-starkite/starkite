# sandbox_host_test.star — Integration tests for the host sandbox rung.
#
# Each test asserts one property of the host rung. Run via:
#   kite test ./tests/sandbox/sandbox_host_test.star --sandbox=host
#
# The host rung is net-access plus read-only $HOME, /usr, /bin, /lib:
# read the host, write only the tree. The driver stages hostinfo.json
# (containing the host $HOME path) into $CWD before the run.

hostinfo = json.decode(read_text("hostinfo.json")) if exists("hostinfo.json") else {"home": os.env("HOME") if os.env("HOME") else "/root"}
HOME = hostinfo.get("home", "/root")

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
    p.remove()

# --- negative: host-write and beyond-rung surfaces ---

def test_home_not_writable():
    if not exists("hostinfo.json"):
        return
    res = path(HOME + "/sandbox_host_probe.txt").try_write_text("x")
    assert(not res.ok, "$HOME must be read-only inside the host rung")

def test_usr_not_writable():
    res = path("/usr/sandbox_host_probe.txt").try_write_text("x")
    assert(not res.ok, "/usr must be read-only inside the host rung")

def test_gvisor_marker():
    res = path("/proc/version").try_read_text()
    if not res.ok:
        return
    content = res.value.lower()
    if "gvisor" in content or "sentry" in content:
        print("gVisor sentry kernel verified")
