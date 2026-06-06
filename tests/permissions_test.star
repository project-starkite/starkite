# Permission System Integration Tests
#
# These tests exercise gated capabilities across the module surface. The test
# runner applies one profile to the whole file via --permissions. Run with:
#
#   kite test ./tests/permissions_test.star --permissions=allow-all   # every test passes
#   kite test ./tests/permissions_test.star --permissions=allow-fs    # only fs/env tests pass
#   kite test ./tests/permissions_test.star --permissions=deny-all    # only pure-compute tests pass
#
# The capability ladder, each profile a superset of the prior:
#   deny-all     pure compute, print, log — no fs, net, or exec
#   allow-fs     + fs.read (any file), fs.write/delete ($CWD only), os.env, io.prompt
#   allow-net    + http.client, ssh
#   allow-local  + http.server, $CWD-scoped os.exec, ai/k8s/mcp
#   allow-all    + unrestricted os.exec, k8s.exec, process control

# Test 1: Pure utility modules should always work
def test_pure_utilities():
    """Pure utility modules work in any mode"""
    # strings
    assert("hello".upper() == "HELLO", "upper() failed")
    assert("HELLO".lower() == "hello", "lower() failed")

    # json
    encoded = json.encode({"key": "value"})
    assert('"key"' in encoded, "json.encode failed")
    decoded = json.decode('{"a": 1}')
    assert(decoded["a"] == 1, "json.decode failed")

    # yaml
    yaml_str = yaml.encode({"foo": "bar"})
    assert("foo:" in yaml_str, "yaml.encode failed")

    # base64
    enc = base64.text("hello").encode()
    assert(base64.text(enc).decode() == b"hello", "base64 roundtrip failed")

    # hash
    h = hash.text("test").sha256()
    assert(len(h) == 64, "sha256 should produce 64 char hex string")

    # uuid
    u = uuid.v4()
    assert(len(u) == 36, "uuid should be 36 chars")

    # time
    now = time.now()
    assert(now.year >= 2024, "time.now should return current time")

    # regexp
    assert(regexp.match(r"\d+", "123"), "regexp should match digits")

# Test 2: Path manipulation (no I/O) should always work
def test_path_functions():
    """Path manipulation functions (no I/O) work in any mode"""
    # These path functions don't do I/O, just path manipulation
    assert((path("a") / "b").string == "a/b", "path join failed")
    assert(path("/path/to/file.txt").name == "file.txt", "path name failed")
    assert(path("/path/to/file.txt").parent.string == "/path/to", "path parent failed")
    assert(path("file.txt").suffix == ".txt", "path suffix failed")
    assert(path("a//b/../c").clean().string == "a/c", "path clean failed")

# Test 3: fmt module should always work
def test_fmt_module():
    """fmt module works in any mode"""
    s = sprintf("Hello %s, number %d", "world", 42)
    assert(s == "Hello world, number 42", "sprintf failed")

# Test 4: Core info functions (read-only system info) should work
def test_core_info():
    """Core info functions work in any mode"""
    h = hostname()
    assert(len(h) > 0, "hostname() should return non-empty string")

    c = cwd()
    assert(len(c) > 0, "cwd() should return non-empty string")

    u = username()
    assert(len(u) > 0, "username() should return non-empty string")

# Test 5: Environment variables — gated under os.env (allow-fs and up)
def test_env_access():
    """Environment access requires allow-fs or higher"""
    home = env("HOME")
    assert(len(home) > 0, "HOME should be set")

    # PATH is usually set
    path = env("PATH")
    assert(len(path) > 0, "PATH should be set")

# Test 6: File read — gated under fs.read (allow-fs and up)
def test_file_read():
    """File read requires allow-fs or higher"""
    # Read this test file itself
    content = read_text("tests/permissions_test.star")
    assert("Permission System Integration Tests" in content, "should read this file")

# Test 7: Command execution — system binary requires allow-all
def test_exec():
    """Command execution of a system binary requires allow-all"""
    output = exec("echo hello")
    assert("hello" in output, "should capture output")

# Test 8: File existence check — gated under fs.read (allow-fs and up)
def test_file_exists():
    """File existence check requires allow-fs or higher"""
    assert(exists("tests/permissions_test.star"), "this file should exist")
    assert(not exists("nonexistent-file-12345.txt"), "nonexistent file should not exist")

# Test 9: Retry module (utility, should always work)
def test_retry():
    """Retry module works in any mode"""
    call_count = {"n": 0}

    def succeed_on_second():
        call_count["n"] += 1
        if call_count["n"] < 2:
            fail("not yet")
        return "success"

    # This will retry and eventually succeed
    # retry.do takes (func, max_attempts?, delay?) where delay is a duration string
    result = retry.do(succeed_on_second, max_attempts=3, delay="10ms")
    assert(call_count["n"] >= 2, "should have retried")

# Test 10: Concur module (utility, should always work)
def test_concur():
    """Concur module works in any mode"""
    def identity(x):
        return x

    # concur.map takes (items, func)
    results = concur.map([1, 2, 3], identity)
    assert(len(results) == 3, "concur.map should return 3 results")
    assert(1 in results and 2 in results and 3 in results, "should contain all values")

# --- Category coverage tests ---
#
# Each test below exercises one gated category. It passes only when the
# active profile grants that category, proving the category is correctly gated.

# Test 11: fs.write category. The path is under /tmp (outside $CWD), so it
# requires allow-all; allow-fs only permits writes within $CWD.
def test_fs_write_category():
    """fs.write outside $CWD requires allow-all"""
    tmp_path = path("/tmp/starkite_perm_write_test")
    tmp_path.write_text("hello")
    assert(tmp_path.exists(), "file should have been written")
    tmp_path.remove()

# Test 12: fs.delete category. Path under /tmp (outside $CWD) → requires allow-all.
def test_fs_delete_category():
    """fs.delete outside $CWD requires allow-all"""
    tmp_path = path("/tmp/starkite_perm_delete_test")
    tmp_path.write_text("temp")
    tmp_path.remove()
    assert(not tmp_path.exists(), "file should have been deleted")

# Test 13: os.env category — setenv (requires allow-fs or higher)
def test_os_env_setenv():
    """os.env category covers both env and setenv"""
    setenv("STARKITE_TEST_VAR", "value")
    assert(env("STARKITE_TEST_VAR") == "value", "setenv should have stuck")

# Test 14: os.process category — chdir (requires allow-all)
def test_os_process_chdir():
    """os.process category covers chdir/exit"""
    original = cwd()
    chdir("/tmp")
    assert(cwd() == "/tmp" or cwd() == "/private/tmp", "chdir should have changed cwd")
    chdir(original)

# Test 15: http.client category — url construction triggers no permission
# but the .get() method is gated. Verify http.config, gated under the client
# category (requires allow-net or higher).
def test_http_client_config():
    """http.client category gates http.config"""
    http.config(timeout="5s")

# Test 16: http.server category — server construction (requires allow-local or higher)
def test_http_server_construct():
    """http.server category is distinct from http.client"""
    srv = http.server(port=0)
    assert(srv != None, "server should construct")

# Test 17: $CWD fs round-trip — read/write/delete in one test (requires allow-fs).
def test_cwd_fs_roundtrip():
    """fs read/write/delete inside $CWD requires allow-fs or higher"""
    p = path(cwd()) / "starkite_cwd_roundtrip.tmp"
    p.write_text("ok")
    assert(p.exists(), "should write under $CWD")
    assert(p.read_text() == "ok", "should read under $CWD")
    p.remove()
    assert(not p.exists(), "should delete under $CWD")
