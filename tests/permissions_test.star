# Permission System Integration Tests
#
# These tests verify the permission system works correctly when executed
# with different permission modes. Run with:
#
#   kite test ./tests/permissions_test.star           # Default trusted mode - all tests pass
#   kite test ./tests/permissions_test.star --sandbox # Sandbox mode - I/O tests should fail

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

# Test 5: Environment variables (when allowed)
# Note: In sandbox mode, env() will fail
def test_env_access():
    """Environment access works in trusted mode"""
    # This test only passes in trusted mode
    home = env("HOME")
    assert(len(home) > 0, "HOME should be set")

    # PATH is usually set
    path = env("PATH")
    assert(len(path) > 0, "PATH should be set")

# Test 6: File read (when allowed)
# Note: In sandbox mode, read_file will fail
def test_file_read():
    """File read works in trusted mode"""
    # Read this test file itself
    content = read_text("tests/permissions_test.star")
    assert("Permission System Integration Tests" in content, "should read this file")

# Test 7: Command execution (when allowed)
# Note: In sandbox mode, exec will fail
def test_exec():
    """Command execution works in trusted mode"""
    output = exec("echo hello")
    assert("hello" in output, "should capture output")

# Test 8: File existence check (when allowed)
# Note: In sandbox mode, exists will fail
def test_file_exists():
    """File existence check works in trusted mode"""
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
