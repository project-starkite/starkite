# fmt_test.star - Tests for fmt module

def test_printf_exists():
    """Test printf function exists."""
    assert(type(printf) == "builtin_function_or_method", "printf should be a function")

def test_sprintf_basic():
    """Test sprintf formats strings."""
    result = sprintf("hello %s", "world")
    assert(result == "hello world", "sprintf should format string")

def test_sprintf_multiple_args():
    """Test sprintf with multiple arguments."""
    result = sprintf("%s has %d items", "list", 5)
    assert(result == "list has 5 items", "sprintf should handle multiple args")

def test_sprintf_no_args():
    """Test sprintf with no format args."""
    result = sprintf("plain string")
    assert(result == "plain string", "sprintf should handle plain string")

def test_sprintf_integers():
    """Test sprintf with integers."""
    result = sprintf("count: %d", 42)
    assert(result == "count: 42", "sprintf should format integers")

def test_sprintf_floats():
    """Test sprintf with floats."""
    result = sprintf("value: %.2f", 3.14159)
    assert("3.14" in result, "sprintf should format floats")

def test_fmt_module_access():
    """Test fmt module can be accessed."""
    result = fmt.sprintf("test %s", "value")
    assert(result == "test value", "fmt.sprintf should work")

def test_println_exists():
    """Test println function exists as global alias."""
    assert(type(println) == "builtin_function_or_method", "println should be a function")

def test_println_no_args():
    """Test println with no arguments (prints empty line)."""
    println()

def test_println_single_arg():
    """Test println with single argument."""
    println("hello")

def test_println_multiple_args():
    """Test println with multiple arguments."""
    println("hello", "world", 42)

def test_fmt_println():
    """Test fmt.println module access."""
    fmt.println("via module")
