# io_test.star - Tests for io module
# Note: confirm and prompt are interactive and can't be easily tested automatically.
# We just verify they exist and have the correct type.

def test_confirm_exists():
    """Test io.confirm function exists."""
    assert(type(io.confirm) == "builtin_function_or_method", "io.confirm should be a function")

def test_prompt_exists():
    """Test io.prompt function exists."""
    assert(type(io.prompt) == "builtin_function_or_method", "io.prompt should be a function")
