# try_test.star - Tests for try_ prefix feature (Result type, TryWrap, TryModule)

# ===========================================================================
# Result type and attributes
# ===========================================================================

def test_try_type():
    """type(os.try_cwd()) == 'Result'"""
    result = os.try_cwd()
    assert(type(result) == "Result", "type should be 'Result', got '%s'" % type(result))

def test_try_attr_access():
    """.ok, .value, .error all accessible"""
    result = os.try_cwd()
    _ = result.ok
    _ = result.value
    _ = result.error

def test_try_string_repr():
    """str(result) contains 'Result(ok=True' / 'Result(ok=False'"""
    ok_result = os.try_cwd()
    assert("Result(ok=True" in str(ok_result), "ok result should contain 'Result(ok=True', got '%s'" % str(ok_result))

    fail_result = path("/nonexistent/xyz").try_read_text()
    assert("Result(ok=False" in str(fail_result), "fail result should contain 'Result(ok=False', got '%s'" % str(fail_result))

# ===========================================================================
# os module try_
# ===========================================================================

def test_try_exec_success():
    """os.try_exec('echo hello') -> ok, value.stdout has 'hello'"""
    result = os.try_exec("echo hello")
    assert(result.ok, "try_exec('echo hello') should succeed")
    assert("hello" in result.stdout, "stdout should contain 'hello', got '%s'" % result.stdout)

def test_try_exec_failure():
    """os.try_exec('false') -> Result(ok=True, value=ExecResult(ok=False, code=1))"""
    result = os.try_exec("false")
    assert(not result.ok, "ExecResult.ok should be False for non-zero exit")
    assert(result.code != 0, "exit code should be non-zero")

def test_try_env_with_args():
    """os.try_env('HOME') succeeds, os.try_env('MISSING', 'fb') returns 'fb'"""
    result = os.try_env("HOME")
    assert(result.ok, "try_env('HOME') should succeed")
    assert(result.value != "", "HOME should be non-empty")

    result2 = os.try_env("STARKITE_NONEXISTENT_XYZ_12345", "fb")
    assert(result2.ok, "try_env with default should succeed")
    assert(result2.value == "fb", "should return default 'fb', got '%s'" % result2.value)

def test_try_chdir_success():
    """os.try_chdir('/tmp') -> ok"""
    original = cwd()
    result = os.try_chdir("/tmp")
    assert(result.ok, "try_chdir('/tmp') should succeed")
    chdir(original)  # restore

def test_try_which_missing():
    """os.try_which('nonexistent_cmd_xyz') -> ok=True, value=None"""
    result = os.try_which("nonexistent_cmd_xyz_12345")
    assert(result.ok, "try_which for missing cmd should still return ok=True (which returns None, not error)")
    assert(result.value == None, "value should be None for missing command")

# ===========================================================================
# fs module try_
# ===========================================================================

def test_try_read_nonexistent():
    """fs.try_read_text('/no/such/file') -> ok=False, error has 'no such file'"""
    result = path("/no/such/file").try_read_text()
    assert(not result.ok, "should be ok=False for nonexistent file")
    assert("no such file" in result.error, "error should mention 'no such file', got '%s'" % result.error)

def test_try_read_success():
    """fs.try_read_text on real file -> ok=True, content in value"""
    result = path("/etc/hosts").try_read_text()
    assert(result.ok, "try_read_text('/etc/hosts') should succeed, error: %s" % result.error)
    assert(len(result.value) > 0, "should have non-empty content")

def test_try_mkdir_and_remove():
    """fs.try_mkdir + fs.try_remove round-trip"""
    test_dir = "/tmp/starkite_try_test_dir"

    # Create
    result = path(test_dir).try_mkdir()
    assert(result.ok, "try_mkdir should succeed, error: %s" % result.error)
    assert(exists(test_dir), "directory should exist after mkdir")

    # Remove
    result = path(test_dir).try_remove()
    assert(result.ok, "try_remove should succeed, error: %s" % result.error)
    assert(not exists(test_dir), "directory should not exist after remove")

# ===========================================================================
# http module try_
# ===========================================================================

def test_try_http_success():
    """http.url(url).try_get() -> ok=True, value has status_code"""
    def handler(req):
        return {"status": 200, "body": "try-ok"}

    srv = http.server()
    srv.handle("/try", handler)
    srv.start(port=0)
    port = srv.port()

    result = http.url("http://localhost:%d/try" % port).try_get()
    assert(result.ok, "try_get should succeed, error: %s" % result.error)
    assert(result.value.status_code == 200, "should get 200")
    assert(result.value.get_text() == "try-ok", "body should be 'try-ok'")

    srv.shutdown()

def test_try_http_connection_error():
    """http.url('http://127.0.0.1:1').try_get(timeout="1s") -> ok=False"""
    result = http.url("http://127.0.0.1:1").try_get(timeout="1s")
    assert(not result.ok, "try_get to unreachable port should be ok=False")
    assert(result.error != "", "should have non-empty error message")

# ===========================================================================
# Control flow
# ===========================================================================

def test_try_truth_shorthand():
    """`if result:` works for both success and failure"""
    ok_result = os.try_cwd()
    if ok_result:
        pass  # expected path
    else:
        fail("ok result should be truthy")

    fail_result = path("/nonexistent/xyz").try_read_text()
    if fail_result:
        fail("error result should be falsy")
    else:
        pass  # expected path
