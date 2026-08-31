# os_test.star - Tests for os module

def test_env_existing():
    """Test env returns existing variable."""
    # HOME on Unix, USERPROFILE or SystemRoot on Windows
    home_dir = env("HOME", env("USERPROFILE", env("SystemRoot", "default")))
    assert(home_dir != "", "standard environment variable should be set")
    assert(home_dir != None, "standard environment variable should not be None")

def test_env_missing_with_default():
    """Test env returns default for missing variable."""
    result = env("STARKITE_NONEXISTENT_VAR_12345", "default_value")
    assert(result == "default_value", "should return default for missing var")

def test_env_missing_no_default():
    """Test env returns empty string for missing variable without default."""
    result = env("STARKITE_NONEXISTENT_VAR_12345")
    assert(result == "", "should return empty string for missing var")

def test_setenv():
    """Test setenv sets environment variable."""
    setenv("STARKITE_TEST_VAR", "test_value")
    result = env("STARKITE_TEST_VAR")
    assert(result == "test_value", "setenv should set variable")

def test_cwd():
    """Test cwd returns current directory."""
    result = cwd()
    assert(result != "", "cwd should return non-empty string")
    assert("/" in result or "\\" in result or ":" in result, "cwd should be an absolute path")

def test_chdir():
    """Test chdir changes working directory."""
    original = cwd()
    parent_dir = fs.path(original).parent.string
    chdir(parent_dir)
    current = cwd()
    assert(len(current) > 0, "cwd should not be empty after chdir")
    chdir(original)
    assert(cwd() == original, "should return to original directory")

def test_hostname():
    """Test hostname returns system hostname."""
    result = hostname()
    assert(result != "", "hostname should return non-empty string")

def test_pid():
    """Test pid returns current process ID."""
    result = pid()
    assert(type(result) == "int", "pid should return int")
    assert(result > 0, "pid should be positive")

def test_ppid():
    """Test ppid returns parent process ID."""
    result = ppid()
    assert(type(result) == "int", "ppid should return int")
    assert(result > 0, "ppid should be positive")
    assert(result != pid(), "ppid should differ from pid")

def test_exec_simple():
    """Test exec with simple command returns string."""
    if runtime.platform() == "windows":
        output = exec("cmd.exe", ["/c", "echo hello"])
    else:
        output = exec("echo", ["hello"])
    assert("hello" in output, "output should contain hello")

def test_exec_with_args():
    """Test exec with arguments."""
    if runtime.platform() == "windows":
        output = exec("cmd.exe", ["/c", "echo test"])
    elif which("printf"):
        output = exec("printf", ["test"])
    else:
        output = exec("echo", ["test"])
    assert("test" in output, "should output test")

def test_exec_failure():
    """Test exec with failing command via try_exec."""
    if runtime.platform() == "windows":
        result = try_exec("cmd.exe", ["/c", "exit 1"])
    else:
        result = try_exec("false")
    assert(not result.ok, "ExecResult.ok should be False for non-zero exit")
    assert(result.code != 0, "exit code should be non-zero")

def test_exec_stderr():
    """Test exec captures stderr via try_exec."""
    if runtime.platform() == "windows":
        result = try_exec("cmd.exe", ["/c", "echo error 1>&2"])
    else:
        result = try_exec("sh", ["-c", "echo error >&2"])
    assert(result.ok, "command should succeed")
    assert("error" in result.stderr or "error" in result.stdout, "should capture error output")

def test_exec_exit_code():
    """Test exec captures specific exit code via try_exec."""
    if runtime.platform() == "windows":
        result = try_exec("cmd.exe", ["/c", "exit 42"])
    else:
        result = try_exec("sh", ["-c", "exit 42"])
    assert(not result.ok, "ExecResult.ok should be False")
    assert(result.code == 42, "should capture non-zero exit code")

def test_exec_with_env():
    """Test exec with environment variables."""
    if runtime.platform() == "windows":
        output = exec("cmd.exe", ["/c", "echo %MY_TEST_VAR%"], env={"MY_TEST_VAR": "test_value"})
    else:
        output = exec("sh", ["-c", "echo $MY_TEST_VAR"], env={"MY_TEST_VAR": "test_value"})
    assert("test_value" in output, "should handle env var")

def test_exec_with_cwd():
    """Test exec with working directory."""
    orig = cwd()
    if runtime.platform() == "windows":
        output = exec("cmd.exe", ["/c", "cd"], cwd=orig)
    else:
        output = exec("pwd", cwd=orig)
    assert(len(output) > 0, "should run in cwd")

def test_which():
    """Test which finds executable."""
    bin_name = "cmd.exe" if which("cmd.exe") else ("sh" if which("sh") else "bash")
    result = which(bin_name)
    assert(result != None and result != "", "should find standard shell executable")
    assert(len(result) > 0, "path should not be empty")

def test_which_missing():
    """Test which with missing command."""
    result = which("nonexistent_command_12345")
    assert(result == None, "should return None for missing command")

def test_username():
    """Test username returns current user."""
    result = username()
    assert(result != "", "username should return non-empty string")

def test_userid():
    """Test userid returns user ID."""
    result = userid()
    assert(result != "", "userid should return non-empty string")

def test_groupid():
    """Test groupid returns group ID."""
    result = groupid()
    assert(result != "", "groupid should return non-empty string")

def test_home():
    """Test home returns home directory."""
    result = home()
    assert(result != "", "home should return non-empty string")
    assert("/" in result or "\\" in result or ":" in result, "home should be an absolute path")

def test_user_alias():
    """Test user alias struct."""
    assert(user.name() == username(), "user.name should equal username")
    assert(user.id() == userid(), "user.id should equal userid")
    assert(user.gid() == groupid(), "user.gid should equal groupid")
    assert(user.home() == home(), "user.home should equal home")

def test_exec_streaming_string_input():
    """Test exec with string input."""
    if not which("cat"):
        return
    output = exec("cat", input="hello from string")
    assert(output == "hello from string", "output should be 'hello from string', got '%s'" % output)

def test_exec_streaming_bytes_input():
    """Test exec with bytes input."""
    if not which("cat"):
        return
    output = exec("cat", input=bytes("hello from bytes"))
    assert(output == "hello from bytes", "output should be 'hello from bytes', got '%s'" % output)

def test_exec_streaming_pipe_files():
    """Test piping streams between files and processes."""
    if not which("cat"):
        return
    in_path = fs.path("starkite_in.txt").resolve()
    out_path = fs.path("starkite_out.txt").resolve()
    
    if in_path.exists():
        in_path.remove()
    if out_path.exists():
        out_path.remove()
        
    in_path.write_text("stream file data")
    
    r = in_path.get_reader()
    w = out_path.get_writer()
    
    res = exec("cat", input=r, output=w)
    assert(res == "", "exec with redirected output should return empty, got '%s'" % res)
    
    assert(out_path.exists(), "output file should exist")
    assert(out_path.read_text() == "stream file data", "output file content should match")
    
    in_path.remove()
    out_path.remove()

def test_try_exec_streaming_pipe_files():
    """Test try_exec with piping streams between files and processes."""
    if not which("cat"):
        return
    in_path = fs.path("starkite_in_try.txt").resolve()
    out_path = fs.path("starkite_out_try.txt").resolve()
    
    if in_path.exists():
        in_path.remove()
    if out_path.exists():
        out_path.remove()
        
    in_path.write_text("try_exec stream data")
    
    r = in_path.get_reader()
    w = out_path.get_writer()
    
    res = try_exec("cat", input=r, output=w)
    assert(res.ok, "try_exec should succeed")
    assert(res.code == 0, "exit code should be 0")
    assert(res.stdout == "", "stdout should be empty due to redirection")
    
    assert(out_path.exists(), "output file should exist")
    assert(out_path.read_text() == "try_exec stream data", "output file content should match")
    
    in_path.remove()
    out_path.remove()

