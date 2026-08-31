#!/usr/bin/env kite
# builtins_test.star - Test built-in functions
#
# Run with: kite test builtins_test.star

# =============================================================================
# STRING TESTS
# =============================================================================

def test_strings_contains():
    """Test string contains using 'in' operator."""
    assert("world" in "hello world", "should contain 'world'")
    assert("foo" not in "hello world", "should not contain 'foo'")

def test_strings_has_prefix():
    """Test string startswith method."""
    assert("hello".startswith("he"), "should have prefix 'he'")
    assert(not "hello".startswith("lo"), "should not have prefix 'lo'")

def test_strings_has_suffix():
    """Test string endswith method."""
    assert("hello".endswith("lo"), "should have suffix 'lo'")
    assert(not "hello".endswith("he"), "should not have suffix 'he'")

def test_strings_upper_lower():
    """Test case conversion functions."""
    assert("hello".upper() == "HELLO", "should uppercase")
    assert("HELLO".lower() == "hello", "should lowercase")

def test_strings_trim():
    """Test string trimming functions."""
    assert("  hello  ".strip() == "hello", "should trim whitespace")
    assert("hello".removeprefix("he") == "llo", "should trim prefix")
    assert("hello".removesuffix("lo") == "hel", "should trim suffix")

def test_strings_split_join():
    """Test split and join functions."""
    parts = "a,b,c".split(",")
    assert(len(parts) == 3, "should split into 3 parts")
    assert(parts[0] == "a", "first part should be 'a'")

    joined = "-".join(["a", "b", "c"])
    assert(joined == "a-b-c", "should join with '-'")

def test_strings_replace():
    """Test string replacement."""
    result = "hello".replace("l", "L")
    assert(result == "heLLo", "should replace all 'l' with 'L'")

# =============================================================================
# REGEXP TESTS
# =============================================================================

def test_regexp_match():
    """Test regexp.match function."""
    assert(regexp.match("^[a-z]+$", "hello"), "should match lowercase letters")
    assert(not regexp.match("^[a-z]+$", "Hello"), "should not match with uppercase")

def test_regexp_find():
    """Test regexp.find function."""
    result = regexp.find("[0-9]+", "abc123def")
    assert(result != None, "should find a match")
    assert(result.text == "123", "should find '123'")

def test_regexp_find_all():
    """Test regexp.find_all function."""
    results = regexp.find_all("[0-9]+", "a1b2c3")
    assert(len(results) == 3, "should find 3 numbers")

# =============================================================================
# JSON TESTS
# =============================================================================

def test_json_encode_decode():
    """Test JSON encoding and decoding."""
    data = {"name": "test", "value": 42}
    encoded = json.encode(data)

    assert("test" in encoded, "encoded should contain 'test'")
    assert("42" in encoded, "encoded should contain '42'")

    decoded = json.decode(encoded)
    assert(decoded["name"] == "test", "decoded name should match")
    assert(decoded["value"] == 42, "decoded value should match")

def test_json_valid():
    """Test JSON validation by decoding."""
    # json module doesn't have valid(), test by trying to decode
    decoded = json.decode('{"key": "value"}')
    assert(decoded["key"] == "value", "should decode valid JSON")
    # Note: invalid JSON would cause an error, so we skip that test

# =============================================================================
# YAML TESTS
# =============================================================================

def test_yaml_encode_decode():
    """Test YAML encoding and decoding."""
    data = {"name": "test", "items": ["a", "b", "c"]}
    encoded = yaml.encode(data)

    decoded = yaml.decode(encoded)
    assert(decoded["name"] == "test", "decoded name should match")
    assert(len(decoded["items"]) == 3, "should have 3 items")

# =============================================================================
# PATH TESTS
# =============================================================================

def test_path_join():
    """Test path join."""
    result = path("var").join("log", "app.log").string
    assert("var" in result, "should contain 'var'")
    assert("log" in result, "should contain 'log'")
    assert("app.log" in result, "should contain 'app.log'")

def test_path_dir_base_ext():
    """Test path manipulation functions."""
    p = path("/var/log/app.log")
    assert(p.name == "app.log", "should get base name")
    assert(p.suffix == ".log", "should get extension")
    assert("var" in p.parent.string and "log" in p.parent.string, "should get directory containing var and log")

def test_path_abs():
    """Test path resolve function."""
    # path.resolve() converts relative to absolute
    result = path("relative").resolve().string
    assert("/" in result or "\\" in result or ":" in result, "should convert to absolute path")
    # Absolute paths retain components
    result2 = path("/var/log").resolve().string
    assert("var" in result2 and "log" in result2, "absolute path should retain components")

# =============================================================================
# TIME TESTS
# =============================================================================

def test_time_now():
    """Test time.now function."""
    now = time.now()
    # Just check that it returns something
    formatted = time.format(now, time.RFC3339)
    assert(len(formatted) > 0, "should return formatted time")

def test_time_duration():
    """Test time.duration function."""
    d = time.duration("5s")
    assert(d.seconds == 5, "should be 5 seconds")

    d2 = time.duration("1m")
    assert(d2.seconds == 60, "should be 60 seconds")

def test_time_comparison():
    """Test time comparison functions."""
    t1 = time.now()
    time.sleep("10ms")  # sleep takes a string, not a duration
    t2 = time.now()

    # Use time.since to check time passed
    elapsed = time.since(t1)
    assert(elapsed.seconds >= 0, "elapsed time should be non-negative")

# =============================================================================
# BASE64 TESTS
# =============================================================================

def test_base64_encode_decode():
    """Test base64 encoding and decoding."""
    original = b"hello world"
    encoded = base64.bytes(original).encode()
    decoded = base64.text(encoded).decode()

    assert(decoded == original, "decoded should match original")
    assert(encoded == "aGVsbG8gd29ybGQ=", "encoding should match expected")

# =============================================================================
# HASH TESTS
# =============================================================================

def test_hash_md5():
    """Test MD5 hashing."""
    result = hash.text("hello").md5()
    assert(result == "5d41402abc4b2a76b9719d911017c592", "MD5 hash should match")

def test_hash_sha256():
    """Test SHA256 hashing."""
    result = hash.text("hello").sha256()
    # SHA256 of "hello"
    assert(len(result) == 64, "SHA256 hash should be 64 characters")

# =============================================================================
# UUID TESTS
# =============================================================================

def test_uuid_new():
    """Test UUID generation."""
    id1 = uuid.v4()
    id2 = uuid.v4()

    assert(id1 != id2, "UUIDs should be unique")
    assert(len(id1) == 36, "UUID should be 36 characters")

def test_uuid_format():
    """Test UUID format."""
    id1 = uuid.v4()
    # UUID format: 8-4-4-4-12
    parts = id1.split("-")
    assert(len(parts) == 5, "UUID should have 5 parts")
    assert(len(parts[0]) == 8, "first part should be 8 chars")
    assert(len(parts[1]) == 4, "second part should be 4 chars")
    assert(len(parts[2]) == 4, "third part should be 4 chars")
    assert(len(parts[3]) == 4, "fourth part should be 4 chars")
    assert(len(parts[4]) == 12, "fifth part should be 12 chars")

# =============================================================================
# LOCAL PROVIDER TESTS
# =============================================================================

def test_local_exec():
    """Test local command execution."""
    cmd = "cmd.exe /c echo hello" if runtime.platform() == "windows" else "echo hello"
    output = exec(cmd)
    assert(output.strip() == "hello", "should output 'hello'")

def test_local_exec_error():
    """Test local command execution with error."""
    cmd = "cmd.exe /c exit 1" if runtime.platform() == "windows" else "false"
    result = try_exec(cmd)
    assert(not result.ok, "ExecResult.ok should be False")
    assert(result.code != 0, "exit code should be non-zero")

def test_local_which():
    """Test which function."""
    bin_name = "cmd.exe" if which("cmd.exe") else ("bash" if which("bash") else "sh")
    found_path = which(bin_name)
    assert(found_path != None, "should find standard shell binary")
    assert(len(found_path) > 0, "path should not be empty")

    not_found = which("nonexistent_command_12345")
    assert(not_found == None, "should return None for nonexistent command")

# =============================================================================
# FORMATTING TESTS
# =============================================================================

def test_sprintf():
    """Test sprintf function."""
    result = sprintf("Hello %s, you have %d messages", "User", 5)
    assert(result == "Hello User, you have 5 messages", "sprintf should format correctly")

def test_env_function():
    """Test env function with default."""
    # This should return the default since the var likely doesn't exist
    value = env("CRSH_TEST_VAR_THAT_DOES_NOT_EXIST", "default_value")
    assert(value == "default_value", "should return default value")
