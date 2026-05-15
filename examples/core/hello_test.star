# hello_test.star — assertions for the helpers used by hello.star.

def test_hostname():
    h = hostname()
    assert(h != "", "hostname should return a non-empty string")

def test_username():
    u = username()
    assert(u != "", "username should return a non-empty string")

def test_cwd():
    d = cwd()
    assert(d != "", "cwd should return a non-empty string")

def test_uname():
    out = os.exec("uname -s").strip()
    assert(out != "", "uname -s should produce output")

def test_env_fallback():
    val = env("__starkite_quickstart_unset_var__", "fallback")
    assert(val == "fallback", "env should return default when var unset")

def test_time_rfc3339():
    s = time.format(time.now(), time.RFC3339)
    assert("T" in s and ":" in s and "-" in s, "RFC3339 timestamp should contain T, :, -")
