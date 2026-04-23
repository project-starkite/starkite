# os_test.star - Tests for os module

def test_env_existing():
    """Test env returns existing variable."""
    home = env("HOME")
    assert(home != "", "HOME should be set")
    assert(home != None, "HOME should not be None")

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
    assert("/" in result, "cwd should be an absolute path")

def test_chdir():
    """Test chdir changes working directory."""
    original = cwd()
    chdir("/tmp")
    # On macOS, /tmp is a symlink to /private/tmp — Go resolves symlinks
    current = cwd()
    assert(current == "/tmp" or current == "/private/tmp", "cwd should be /tmp or /private/tmp after chdir, got %s" % current)
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
    output = exec("echo hello")
    assert(output == "hello\n", "output should be 'hello\\n', got '%s'" % output)

def test_exec_with_args():
    """Test exec with arguments."""
    output = exec("printf test")
    assert(output == "test", "should output without newline")

def test_exec_failure():
    """Test exec with failing command via try_exec."""
    result = try_exec("false")
    assert(not result.ok, "ExecResult.ok should be False for non-zero exit")
    assert(result.code != 0, "exit code should be non-zero")

def test_exec_stderr():
    """Test exec captures stderr via try_exec."""
    result = try_exec("echo error >&2")
    assert(result.ok, "command should succeed")
    assert("error" in result.stderr, "should capture stderr")

def test_exec_exit_code():
    """Test exec captures specific exit code via try_exec."""
    result = try_exec("exit 42")
    assert(not result.ok, "ExecResult.ok should be False")
    assert(result.code == 42, "should capture exit code 42")

def test_exec_with_env():
    """Test exec with environment variables."""
    output = exec("echo $MY_TEST_VAR", env={"MY_TEST_VAR": "test_value"})
    assert("test_value" in output, "should use env var")

def test_exec_with_cwd():
    """Test exec with working directory."""
    output = exec("pwd", cwd="/tmp")
    assert("/tmp" in output, "should run in /tmp")

def test_which():
    """Test which finds executable."""
    result = which("sh")
    assert(result != "", "should find sh")
    assert("sh" in result, "path should contain sh")

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
    assert("/" in result, "home should be an absolute path")

def test_user_alias():
    """Test user alias struct."""
    assert(user.name() == username(), "user.name should equal username")
    assert(user.id() == userid(), "user.id should equal userid")
    assert(user.gid() == groupid(), "user.gid should equal groupid")
    assert(user.home() == home(), "user.home should equal home")
