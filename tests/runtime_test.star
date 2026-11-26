# runtime_test.star - Tests for runtime module

def test_platform():
    """Test runtime.platform returns OS name."""
    result = runtime.platform()
    assert(result != "", "platform should return non-empty string")
    assert(result == "linux" or result == "darwin" or result == "windows",
           "platform should be a known OS")

def test_arch():
    """Test runtime.arch returns CPU architecture."""
    result = runtime.arch()
    assert(result != "", "arch should return non-empty string")
    assert(result in ["amd64", "arm64", "386", "arm"],
           "arch should be a known architecture")

def test_cpu_count():
    """Test runtime.cpu_count returns positive number."""
    result = runtime.cpu_count()
    assert(type(result) == "int", "cpu_count should return int")
    assert(result > 0, "cpu_count should be positive")

def test_uname():
    """Test runtime.uname returns system info struct."""
    result = runtime.uname()
    assert(type(result) == "struct", "uname should return struct")
    assert(result.system != "", "system should not be empty")
    _ = (result.node, result.release, result.version, result.machine)

def test_version():
    """Test runtime.version returns starkite version."""
    result = runtime.version()
    assert(type(result) == "string", "version should return string")

def test_go_version():
    """Test runtime.go_version returns Go version."""
    result = runtime.go_version()
    assert(type(result) == "string", "go_version should return string")
    assert("go" in result, "go_version should contain 'go'")
