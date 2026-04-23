#!/usr/bin/env kite
# sample_test.starctl - Example test file demonstrating test features

# Setup runs before each test
def setup():
    # You can set up test fixtures here
    pass

# Teardown runs after each test (even if test fails)
def teardown():
    # Clean up test fixtures here
    pass

def test_basic_assertion():
    """Test that basic assertions work."""
    assert(1 + 1 == 2, "basic math should work")
    assert(True, "true should be truthy")

def test_string_operations():
    """Test string module functions."""
    result = "hello".upper()
    assert(result == "HELLO", "upper() should uppercase")

    parts = "a,b,c".split(",")
    assert(len(parts) == 3, "split should create 3 parts")

def test_local_exec():
    """Test local command execution."""
    output = os.exec("echo hello")
    assert(output.strip() == "hello", "output should be hello")

def test_exec_failure():
    """Test handling of failed commands."""
    result = os.try_exec("false")
    assert(not result.ok, "false command should fail")
    assert(result.code != 0, "exit code should be non-zero")

def test_skip_example():
    """This test will be skipped."""
    skip("demonstrating skip functionality")
    assert(False, "this should never run")

def test_environment():
    """Test environment variable access."""
    home = env("HOME", "")
    assert(home != "", "HOME should be set")

def test_json_roundtrip():
    """Test JSON encode/decode."""
    data = {"name": "starctl", "version": 1}
    encoded = json.encode(data)
    decoded = json.decode(encoded)
    assert(decoded["name"] == "starctl", "name should survive roundtrip")
