# vars_test.star - Tests for vars module

# --- function existence ---

def test_vars_module_functions():
    """Test vars module exports all expected functions."""
    funcs = dir(vars)
    assert_contains(funcs, "var_str")
    assert_contains(funcs, "var_int")
    assert_contains(funcs, "var_bool")
    assert_contains(funcs, "var_float")
    assert_contains(funcs, "var_names")
    assert_contains(funcs, "var_list")
    assert_contains(funcs, "var_dict")

# --- var_str (renamed from var) ---

def test_var_str_default():
    """var_str returns default when variable doesn't exist."""
    result = var_str("nonexistent_var_str_test", "fallback")
    assert_equal(result, "fallback")

def test_var_str_empty_default():
    """var_str returns empty string when no default given."""
    result = var_str("nonexistent_var_str_test2")
    assert_equal(result, "")

# --- var_int ---

def test_var_int_default():
    """var_int returns default when variable doesn't exist."""
    result = var_int("nonexistent_var_int_test", 42)
    assert_equal(result, 42)

def test_var_int_zero_default():
    """var_int returns 0 when no default given."""
    result = var_int("nonexistent_var_int_test2")
    assert_equal(result, 0)

# --- var_names ---

def test_var_names_returns_list():
    """var_names returns a list."""
    names = var_names()
    assert_equal(type(names), "list")

# --- var_list ---

def test_var_list_default():
    """var_list returns default list when variable doesn't exist."""
    result = var_list("nonexistent_list", [1, 2, 3])
    assert_equal(result, [1, 2, 3])

def test_var_list_empty_default():
    """var_list returns empty list when no default given."""
    result = var_list("nonexistent_list2")
    assert_equal(result, [])

# --- var_dict ---

def test_var_dict_default():
    """var_dict returns default dict when variable doesn't exist."""
    result = var_dict("nonexistent_dict", {"a": "b"})
    assert_equal(result, {"a": "b"})

def test_var_dict_empty_default():
    """var_dict returns empty dict when no default given."""
    result = var_dict("nonexistent_dict2")
    assert_equal(result, {})

# --- global aliases ---

def test_var_str_global():
    """var_str is available as global alias."""
    assert_equal(type(var_str), "builtin_function_or_method")

def test_var_names_global():
    """var_names is available as global alias."""
    assert_equal(type(var_names), "builtin_function_or_method")

def test_var_list_global():
    """var_list is available as global alias."""
    assert_equal(type(var_list), "builtin_function_or_method")

def test_var_dict_global():
    """var_dict is available as global alias."""
    assert_equal(type(var_dict), "builtin_function_or_method")

# --- module-qualified access ---

def test_var_str_module():
    """vars.var_str works via module."""
    result = vars.var_str("nonexistent_module_test", "mod_default")
    assert_equal(result, "mod_default")

def test_var_list_module():
    """vars.var_list works via module."""
    result = vars.var_list("nonexistent_module_test2", [42])
    assert_equal(result, [42])

def test_var_dict_module():
    """vars.var_dict works via module."""
    result = vars.var_dict("nonexistent_module_test3", {"k": "v"})
    assert_equal(result, {"k": "v"})
