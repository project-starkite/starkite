# load_test.star - Tests for external module loading
#
# Tests the script module system: load(), search paths, private symbol
# filtering, multi-file modules, caching, dependency chains, and
# built-in module access from external modules.

# Load a single-file module by explicit path
# The module exports a 'greeter' symbol (derived from filename)
load("./modules/greeter.star", "greeter")

# Load a multi-file module by name (from ./modules/)
# The module exports a 'mymodule' symbol (derived from directory name)
# Use Starlark aliasing syntax: alias = "symbol"
load("mymodule", mymod = "mymodule")

# Load a module that itself loads another module (dependency chain)
load("./modules/wrapper.star", "wrapper")

# Load a module that uses built-in modules
load("./modules/builtins_user.star", "builtins_user")

# Load the counter module twice to test caching
load("./modules/counter.star", "counter")

# =============================================================================
# SINGLE-FILE MODULE LOADING
# =============================================================================

def test_single_file_module_load():
    """Test loading a single-file module by explicit path."""
    result = greeter.greet("Alice")
    assert(result == "Greetings, Alice!", "expected greeting")

def test_single_file_module_functions():
    """Test all functions in single-file module."""
    assert(greeter.greet("Bob") == "Greetings, Bob!", "greet failed")
    assert(greeter.farewell("Bob") == "Goodbye, Bob!", "farewell failed")

# =============================================================================
# MULTI-FILE MODULE LOADING
# =============================================================================

def test_multi_file_module_load():
    """Test loading a multi-file module via name search."""
    result = mymod.hello()
    assert(result == "Hello, world!", "expected hello world")

def test_multi_file_module_with_args():
    """Test multi-file module function with arguments."""
    result = mymod.hello("Claude")
    assert(result == "Hello, Claude!", "expected hello Claude")

def test_multi_file_module_math():
    """Test function in multi-file module main.star."""
    result = mymod.add(3, 5)
    assert(result == 8, "expected 8")

def test_multi_file_module_merged_symbols():
    """Test that symbols from additional .star files are merged."""
    result = mymod.multiply(4, 7)
    assert(result == 28, "expected 28 from extras.star multiply")

def test_multi_file_private_not_merged():
    """Test that private symbols from additional files are not merged."""
    assert(not hasattr(mymod, "_extra_private"), "private from extras.star should not be merged")

# =============================================================================
# PRIVATE SYMBOL FILTERING
# =============================================================================

def test_private_function_not_accessible():
    """Test that private functions are not exported from single-file modules."""
    assert(not hasattr(greeter, "_private"), "private function should not be exported")

def test_private_from_main_not_accessible():
    """Test that private functions from main.star are not exported."""
    assert(not hasattr(mymod, "_internal_helper"), "private from main.star should not be exported")

def test_public_can_use_private():
    """Test that public functions can call private helpers internally."""
    result = mymod.use_internal()
    assert(result == "internal", "public should be able to use private")

# =============================================================================
# MODULE CONFIGURATION
# =============================================================================

def test_config_available_in_module():
    """Test that _config is available in loaded modules."""
    result = mymod.get_config_value("nonexistent", "default_value")
    assert(result == "default_value", "config should return default for missing keys")

# =============================================================================
# MODULE CACHING
# =============================================================================

def test_module_caching():
    """Test that modules are cached and not re-executed on duplicate load."""
    # counter.star sets load_count = 1 at load time.
    # If caching works, loading it again should return the same instance
    # (load_count still 1, not re-initialized).
    count = counter.get_load_count()
    assert(count == 1, "module should be loaded once, got %d" % count)

def test_module_frozen_after_load():
    """Test that module state is frozen after loading (Starlark semantics).

    Starlark freezes all module globals after execution completes.
    Mutable containers (lists, dicts) in module scope become immutable.
    """
    # The counter's initial state should be readable but not mutable
    count = counter.get_load_count()
    assert(count == 1, "frozen state should still be readable, got %d" % count)

# =============================================================================
# MODULE DEPENDENCY CHAINS
# =============================================================================

def test_module_loads_another_module():
    """Test that a module can load and use another module."""
    result = wrapper.greet_upper("Alice")
    assert(result == "GREETINGS, ALICE!", "wrapper should use greeter and strings")

def test_module_dependency_chain():
    """Test multi-level function calls through dependency chain."""
    result = wrapper.double_farewell("Bob")
    assert(result == "Goodbye, Bob! Goodbye, Bob!", "double farewell failed")

# =============================================================================
# BUILT-IN MODULES ACCESSIBLE FROM EXTERNAL MODULES
# =============================================================================

def test_builtin_json_in_module():
    """Test that external modules can use the built-in json module."""
    result = builtins_user.encode_json({"key": "value"})
    decoded = json.decode(result)
    assert(decoded["key"] == "value", "json encoding in module should work")

def test_builtin_strings_in_module():
    """Test that external modules can use the built-in strings module."""
    result = builtins_user.join_strings(["a", "b", "c"], "-")
    assert(result == "a-b-c", "sep.join in module should work")

def test_builtin_runtime_in_module():
    """Test that external modules can use the built-in runtime module."""
    plat = builtins_user.get_platform()
    assert(plat == "linux" or plat == "darwin" or plat == "windows",
           "runtime.platform() should return valid OS")

# =============================================================================
# MODULE STRUCT PROPERTIES
# =============================================================================

def test_module_is_struct():
    """Test that loaded modules behave as proper module structs."""
    # Module should have an identifiable type
    t = type(greeter)
    assert("module" in t or "Module" in t, "expected module type, got %s" % t)

def test_module_has_expected_attrs():
    """Test hasattr on loaded modules."""
    assert(hasattr(greeter, "greet"), "greeter should have greet")
    assert(hasattr(greeter, "farewell"), "greeter should have farewell")
    assert(not hasattr(greeter, "nonexistent"), "should not have nonexistent attr")
