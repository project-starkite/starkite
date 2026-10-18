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
    cmd = "cmd.exe /c echo hello" if runtime.platform() == "windows" else "echo hello"
    result = os.try_exec(cmd)
    assert(result.ok, "try_exec('%s') should succeed" % cmd)
    assert("hello" in result.stdout, "stdout should contain 'hello', got '%s'" % result.stdout)

def test_try_exec_failure():
    """os.try_exec('false') -> Result(ok=True, value=ExecResult(ok=False, code=1))"""
    cmd = "cmd.exe /c exit 1" if runtime.platform() == "windows" else "false"
    result = os.try_exec(cmd)
    assert(not result.ok, "ExecResult.ok should be False for non-zero exit")
    assert(result.code != 0, "exit code should be non-zero")

def test_try_env_with_args():
    """os.try_env('HOME') succeeds, os.try_env('MISSING', 'fb') returns 'fb'"""
    var_name = "USERPROFILE" if runtime.platform() == "windows" else "HOME"
    result = os.try_env(var_name)
    assert(result.ok, "try_env('%s') should succeed" % var_name)
    assert(result.value != "", "%s should be non-empty" % var_name)

    result2 = os.try_env("STARKITE_NONEXISTENT_XYZ_12345", "fb")
    assert(result2.ok, "try_env with default should succeed")
    assert(result2.value == "fb", "should return default 'fb', got '%s'" % result2.value)

def test_try_chdir_success():
    """os.try_chdir(temp_dir) -> ok"""
    original = cwd()
    temp_dir = env("TEMP", env("TMP", "/tmp"))
    result = os.try_chdir(temp_dir)
    assert(result.ok, "try_chdir('%s') should succeed" % temp_dir)
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
    assert("no such file" in result.error or "cannot find" in result.error, "error should mention 'no such file', got '%s'" % result.error)

def test_try_read_success():
    """fs.try_read_text on real file -> ok=True, content in value"""
    target = "C:/Windows/win.ini" if runtime.platform() == "windows" else "/etc/hosts"
    result = path(target).try_read_text()
    assert(result.ok, "try_read_text('%s') should succeed, error: %s" % (target, result.error))
    assert(len(result.value) > 0, "should have non-empty content")

def test_try_mkdir_and_remove():
    """fs.try_mkdir + fs.try_remove round-trip"""
    temp_dir = env("TEMP", env("TMP", "/tmp"))
    test_dir = (path(temp_dir) / "starkite_try_test_dir").string

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

# ===========================================================================
# Stdlib and Dynamic Value-Type try_ contracts (v0.9.5)
# ===========================================================================

def test_try_stdlib_modules():
    """Verify universal try_ contract on stdlib modules"""
    res1 = strings.try_ljust("a", 3, fillchar="xy")
    assert(not res1.ok, "fillchar > 1 rune should return ok=False")

    res2 = strings.try_cut("a=b", "=")
    assert(res2.ok, "strings.try_cut should return ok=True")
    assert(res2.value.before == "a", "before should be 'a'")
    assert(res2.value.after == "b", "after should be 'b'")

    res3 = time.try_parse("date", "not-a-date")
    assert(not res3.ok, "invalid date parse should return ok=False")

    res4 = time.try_now()
    assert(res4.ok, "time.try_now should return ok=True")

    res5 = fmt.try_sprintf("%s %d", "num", 42)
    assert(res5.ok, "fmt.try_sprintf should return ok=True")
    assert(res5.value == "num 42", "formatted output mismatch")

    res6 = runtime.try_version()
    assert(res6.ok, "runtime.try_version should return ok=True")

def test_try_time_and_duration_values():
    """Verify try_ methods and accessors on TimeValue and DurationValue"""
    t = time.now()
    t_add = t.try_add("1h")
    assert(t_add.ok, "try_add('1h') should succeed")

    t_add_bad = t.try_add("invalid-duration-format")
    assert(not t_add_bad.ok, "try_add invalid duration should return ok=False")

    t_str = t.try_string()
    assert(t_str.ok, "try_string on TimeValue should succeed")
    assert(len(t_str.value) > 0, "try_string should produce non-empty representation")

    d = time.duration(10)
    d_str = d.try_string()
    assert(d_str.ok, "try_string on DurationValue should succeed")
    assert(d_str.value == "10s", "duration value should be '10s', got '%s'" % d_str.value)

    assert(t.unix_milli > 0, "unix_milli accessor should return positive integer")
    assert(t.nanosecond >= 0, "nanosecond accessor should return non-negative integer")
    assert(t.utc != None, "utc accessor should return TimeValue")

def test_try_table_and_log_values():
    """Verify try_ methods on TableValue and Logger"""
    tbl = table.new(["col1", "col2"])
    tbl_bad = tbl.try_add_row("a", "b", "c")
    assert(not tbl_bad.ok, "too many values in row should return ok=False")

    tbl_ok = tbl.try_add_row("a")
    assert(tbl_ok.ok, "auto-padded row should return ok=True")

    tbl_rnd = tbl.try_render()
    assert(tbl_rnd.ok, "try_render should return ok=True")
    assert("col1" in tbl_rnd.value, "rendered table should contain col1")

    l = log.logger()
    l_info = l.try_info("test message")
    assert(l_info.ok, "logger.try_info should return ok=True")
