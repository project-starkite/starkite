# test_test.star - Tests for test module

# --- function existence ---

def test_skip_exists():
    """Test skip function exists."""
    assert(type(skip) == "builtin_function_or_method", "skip should be a function")

def test_fail_exists():
    """Test fail function exists."""
    assert(type(fail) == "builtin_function_or_method", "fail should be a function")

def test_test_module_skip():
    """Test test.skip function exists."""
    assert(type(test.skip) == "builtin_function_or_method", "test.skip should be a function")

def test_test_module_fail():
    """Test test.fail function exists."""
    assert(type(test.fail) == "builtin_function_or_method", "test.fail should be a function")

def test_test_module_assert():
    """Test test.assert function exists."""
    assert(type(test.assert) == "builtin_function_or_method", "test.assert should be a function")

# --- assert is unified (global == test.assert) ---

def test_assert_global_is_same():
    """Global assert and test.assert are the same implementation."""
    # Both should accept truthy values, optional msg, format args
    assert(True)
    test.assert(True)
    assert(True, "with message")
    test.assert(True, "with message")

# --- assert with no message ---

def test_assert_no_message():
    """assert(condition) works without msg."""
    assert(True)
    assert(1 == 1)

# --- assert basic ---

def test_assert_passes():
    """assert passes for true condition."""
    assert(True, "this should pass")
    assert(1 == 1, "math should work")
    assert(len("hello") == 5, "string length should be 5")

# --- assert accepts truthy values ---

def test_assert_truthy_values():
    """assert accepts any truthy value, not just Bool."""
    assert("hello", "non-empty string is truthy")
    assert([1, 2], "non-empty list is truthy")
    assert({"a": 1}, "non-empty dict is truthy")
    assert(42, "non-zero int is truthy")
    r = Result(ok=True, value="x")
    assert(r, "Result(ok=True) is truthy")

# --- Starlark str.format() at script-time ---

def test_assert_starlark_format():
    """assert with Starlark "".format() evaluated before the call."""
    x = 42
    assert(x > 0, "x should be positive, got {}".format(x))
    assert(x == 42, "expected {} got {}".format(42, x))

def test_assert_starlark_format_indexed():
    """assert with {0}, {1} indexed format."""
    assert(True, "first={0} second={1}".format("a", "b"))

# --- printf-style format verbs ---

def test_assert_printf_format():
    """assert with %d, %s printf-style format verbs via extra args."""
    x = 42
    test.assert(x > 0, "x should be positive, got %d", x)
    test.assert(x == 42, "x should be %d, got %d", 42, x)

def test_assert_printf_string_verb():
    """assert with %s format verb."""
    name = "world"
    assert(name == "world", "name should be %s", "world")

# --- skip ---

def test_skip_in_test():
    """Test that skip works."""
    skip("intentionally skipping this test")
    # This line should never be reached
    assert(False, "should not reach here")

# --- assert_equal ---

def test_assert_equal_passes():
    assert_equal(1, 1)
    assert_equal("hello", "hello")
    assert_equal([1, 2, 3], [1, 2, 3])
    assert_equal({"a": 1}, {"a": 1})

def test_assert_equal_module():
    test.assert_equal(42, 42)

def test_assert_equal_with_msg():
    assert_equal(42, 42, "should be %d", 42)

# --- assert_not_equal ---

def test_assert_not_equal_passes():
    assert_not_equal(1, 2)
    assert_not_equal("hello", "world")

def test_assert_not_equal_module():
    test.assert_not_equal("a", "b")

# --- assert_contains ---

def test_assert_contains_string():
    assert_contains("hello world", "world")

def test_assert_contains_list():
    assert_contains([1, 2, 3], 2)

def test_assert_contains_dict():
    assert_contains({"a": 1, "b": 2}, "a")

def test_assert_contains_tuple():
    assert_contains((1, 2, 3), 2)

def test_assert_contains_with_msg():
    assert_contains([1, 2, 3], 2, "list should have %d", 2)

# --- assert_true / assert_false ---

def test_assert_true_passes():
    assert_true(True)
    assert_true("non-empty")
    assert_true([1])

def test_assert_true_module():
    test.assert_true(1 == 1)

def test_assert_true_with_msg():
    assert_true(1 == 1, "math should work")

def test_assert_false_passes():
    assert_false(False)
    assert_false(0)
    assert_false("")
    assert_false([])

def test_assert_false_module():
    test.assert_false(1 == 2)
