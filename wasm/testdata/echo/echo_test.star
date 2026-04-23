# echo_test.star - End-to-end Starlark tests for the echo WASM plugin.
#
# These tests exercise the full pipeline: Starlark runtime -> Registry ->
# WasmModule -> Extism plugin -> JSON marshaling -> WASM guest -> JSON
# unmarshaling -> Starlark value.

# --- echo function tests ---

def test_echo_basic():
    """echo returns the input string unchanged."""
    result = echo.echo("hello world")
    assert(result == "hello world", "echo should return input, got: %s" % result)

def test_echo_empty():
    """echo handles empty string."""
    result = echo.echo("")
    assert(result == "", "echo('') should return empty string, got: %s" % result)

def test_echo_unicode():
    """echo handles unicode characters."""
    result = echo.echo("hello 世界 🌍")
    assert(result == "hello 世界 🌍", "echo should preserve unicode, got: %s" % result)

def test_echo_special_chars():
    """echo handles JSON-special characters (quotes, backslashes, newlines)."""
    result = echo.echo('line1\nline2\ttab "quoted"')
    assert(result == 'line1\nline2\ttab "quoted"', "echo should preserve special chars")

def test_echo_long_string():
    """echo handles longer strings (verifies WASM memory management)."""
    long = "abcdefghij" * 100
    result = echo.echo(long)
    assert(result == long, "echo should handle 1000-char string")
    assert(len(result) == 1000, "result length should be 1000, got: %d" % len(result))

def test_echo_kwarg():
    """echo works with keyword argument."""
    result = echo.echo(input="keyword arg")
    assert(result == "keyword arg", "echo should accept kwarg, got: %s" % result)

# --- add function tests ---

def test_add_basic():
    """add returns the sum of two integers."""
    result = echo.add(17, 25)
    assert(result == 42, "add(17, 25) should be 42, got: %d" % result)

def test_add_zero():
    """add handles zero."""
    assert(echo.add(0, 0) == 0, "add(0, 0) should be 0")
    assert(echo.add(5, 0) == 5, "add(5, 0) should be 5")
    assert(echo.add(0, 7) == 7, "add(0, 7) should be 7")

def test_add_negative():
    """add handles negative numbers."""
    assert(echo.add(-3, -7) == -10, "add(-3, -7) should be -10")
    assert(echo.add(-5, 10) == 5, "add(-5, 10) should be 5")
    assert(echo.add(10, -3) == 7, "add(10, -3) should be 7")

def test_add_large():
    """add handles large numbers."""
    result = echo.add(1000000, 2000000)
    assert(result == 3000000, "add(1M, 2M) should be 3M, got: %d" % result)

def test_add_kwargs():
    """add works with keyword arguments."""
    result = echo.add(a=10, b=20)
    assert(result == 30, "add(a=10, b=20) should be 30, got: %d" % result)

def test_add_kwargs_reversed():
    """add kwargs can be in any order."""
    result = echo.add(b=100, a=23)
    assert(result == 123, "add(b=100, a=23) should be 123, got: %d" % result)

def test_add_mixed_args():
    """add works with positional + keyword arguments."""
    result = echo.add(5, b=15)
    assert(result == 20, "add(5, b=15) should be 20, got: %d" % result)

# --- cross-function tests ---

def test_echo_add_composition():
    """Results from add can be converted and echoed."""
    sum = echo.add(3, 4)
    result = echo.echo(str(sum))
    assert(result == "7", "echo(str(add(3,4))) should be '7', got: %s" % result)

def test_multiple_calls():
    """Multiple sequential calls produce correct independent results."""
    r1 = echo.echo("first")
    r2 = echo.add(1, 2)
    r3 = echo.echo("third")
    r4 = echo.add(10, 20)
    assert(r1 == "first", "first echo failed")
    assert(r2 == 3, "first add failed")
    assert(r3 == "third", "second echo failed")
    assert(r4 == 30, "second add failed")

# --- module identity tests ---

def test_module_is_accessible():
    """The echo module is available as a top-level name."""
    assert(echo != None, "echo module should not be None")

def test_module_has_echo():
    """The echo module has an echo member."""
    assert(hasattr(echo, "echo"), "echo module should have 'echo' function")

def test_module_has_add():
    """The echo module has an add member."""
    assert(hasattr(echo, "add"), "echo module should have 'add' function")
