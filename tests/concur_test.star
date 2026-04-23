# concur_test.star - Tests for concur module

# --- concur.map tests ---

def test_map_simple():
    """Test concur.map with simple function."""
    items = [1, 2, 3, 4, 5]

    def double(x):
        return x * 2

    results = concur.map(items, double)
    assert(len(results) == 5, "should have 5 results")
    assert(2 in results, "should contain 2")
    assert(4 in results, "should contain 4")
    assert(10 in results, "should contain 10")

def test_map_with_strings():
    """Test concur.map with string transformation."""
    items = ["a", "b", "c"]

    def upper(s):
        return s.upper()

    results = concur.map(items, upper)
    assert(len(results) == 3, "should have 3 results")
    assert("A" in results, "should contain A")
    assert("B" in results, "should contain B")
    assert("C" in results, "should contain C")

def test_map_empty():
    """Test concur.map with empty list."""
    def identity(x):
        return x

    results = concur.map([], identity)
    assert(len(results) == 0, "should return empty list")

def test_map_single():
    """Test concur.map with single item."""
    def square(x):
        return x * x

    results = concur.map([5], square)
    assert(len(results) == 1, "should have 1 result")
    assert(results[0] == 25, "should be 25")

def test_map_tuple():
    """Test concur.map with tuple input."""
    def add10(x):
        return x + 10

    results = concur.map((1, 2, 3), add10)
    assert(len(results) == 3, "should have 3 results")
    assert(11 in results, "should contain 11")
    assert(12 in results, "should contain 12")
    assert(13 in results, "should contain 13")

def test_map_preserves_order():
    """Test concur.map preserves order."""
    items = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]

    def identity(x):
        return x

    results = concur.map(items, identity)
    for i in range(len(items)):
        assert(results[i] == items[i], "order should be preserved at index %d" % i)

def test_map_preserves_types():
    """Test concur.map preserves return types."""
    items = [1, 2, 3]

    def to_dict(x):
        return {"value": x}

    results = concur.map(items, to_dict)
    for r in results:
        assert("value" in r, "should have value key")

def test_map_workers():
    """Test concur.map with workers kwarg."""
    items = [1, 2, 3, 4, 5]

    def double(x):
        return x * 2

    results = concur.map(items, double, workers=2)
    assert(len(results) == 5, "should have 5 results")
    assert(2 in results, "should contain 2")
    assert(10 in results, "should contain 10")

def test_map_timeout():
    """Test concur.map with timeout (fast function succeeds)."""
    def identity(x):
        return x

    results = concur.map([1, 2, 3], identity, timeout="5s")
    assert(len(results) == 3, "should complete within timeout")

# --- concur.each tests ---

def test_each():
    """Test concur.each runs function on items (side effects only)."""
    items = [1, 2, 3]

    def double(x):
        return x * 2

    result = concur.each(items, func=double)
    assert(result == None, "concur.each returns None")

def test_each_empty():
    """Test concur.each with empty list."""
    def identity(x):
        return x
    result = concur.each([], func=identity)
    assert(result == None, "concur.each returns None")

def test_each_tuple():
    """Test concur.each with tuple input."""
    def noop(x):
        return x

    result = concur.each((1, 2, 3), func=noop)
    assert(result == None, "concur.each returns None")

# --- concur.exec tests ---

def test_exec_basic():
    """Test concur.exec with 3 functions."""
    def fn_a():
        return "a"
    def fn_b():
        return "b"
    def fn_c():
        return "c"

    a, b, c = concur.exec(fn_a, fn_b, fn_c)
    assert(a == "a", "fn_a should return 'a'")
    assert(b == "b", "fn_b should return 'b'")
    assert(c == "c", "fn_c should return 'c'")

def test_exec_single():
    """Test concur.exec with 1 function."""
    def fn():
        return 42

    results = concur.exec(fn)
    assert(len(results) == 1, "should have 1 result")
    assert(results[0] == 42, "should be 42")

def test_exec_empty():
    """Test concur.exec with 0 functions."""
    results = concur.exec()
    assert(len(results) == 0, "should return empty tuple")

def test_exec_timeout():
    """Test concur.exec with timeout (fast function succeeds)."""
    def fn():
        return "done"

    results = concur.exec(fn, timeout="5s")
    assert(results[0] == "done", "should complete within timeout")

# --- on_error="continue" ---

def test_map_on_error_continue():
    """Test concur.map with on_error=continue returns Results."""
    def maybe_fail(x):
        if x == 2:
            fail("item 2 failed")
        return x * 10

    results = concur.map([1, 2, 3], maybe_fail, on_error="continue")
    assert(len(results) == 3, "should have 3 results")

    # Item 0: ok
    assert(results[0].ok, "item 0 should be ok")
    assert(results[0].value == 10, "item 0 value should be 10")

    # Item 1: error
    assert(not results[1].ok, "item 1 should be error")
    assert("item 2 failed" in results[1].error, "item 1 error should mention failure")

    # Item 2: ok
    assert(results[2].ok, "item 2 should be ok")
    assert(results[2].value == 30, "item 2 value should be 30")

def test_map_continue_all_ok():
    """Test concur.map with on_error=continue when all succeed."""
    def identity(x):
        return x

    results = concur.map([1, 2, 3], identity, on_error="continue")
    for i in range(3):
        assert(results[i].ok, "item %d should be ok" % i)

def test_each_on_error_continue():
    """Test concur.each with on_error=continue returns Results."""
    def maybe_fail(x):
        if x == 2:
            fail("boom")
        return x

    results = concur.each([1, 2, 3], maybe_fail, on_error="continue")
    assert(len(results) == 3, "should have 3 results")
    assert(results[0].ok, "item 0 should be ok")
    assert(not results[1].ok, "item 1 should be error")
    assert(results[2].ok, "item 2 should be ok")

def test_exec_on_error_continue():
    """Test concur.exec with on_error=continue returns Results."""
    def ok_fn():
        return 1
    def fail_fn():
        fail("boom")

    results = concur.exec(ok_fn, fail_fn, ok_fn, on_error="continue")
    assert(len(results) == 3, "should have 3 results")
    assert(results[0].ok, "fn 0 should be ok")
    assert(not results[1].ok, "fn 1 should be error")
    assert(results[2].ok, "fn 2 should be ok")

def test_invalid_on_error():
    """Test invalid on_error value fails."""
    def identity(x):
        return x

    result = concur.try_map([1], identity, on_error="bogus")
    assert(not result.ok, "should fail with invalid on_error")

# --- try_ variants ---

def test_try_map_failure():
    """Test try_map wraps error in Result."""
    def always_fail(x):
        fail("boom")

    result = concur.try_map([1, 2], always_fail)
    assert(not result.ok, "should be error result")
    assert("boom" in result.error, "should contain error message")

def test_try_map_success():
    """Test try_map wraps success in Result."""
    def double(x):
        return x * 2

    result = concur.try_map([1, 2, 3], double)
    assert(result.ok, "should be ok result")
    assert(len(result.value) == 3, "value should be list of 3")

def test_try_each_failure():
    """Test try_each wraps error in Result."""
    def always_fail(x):
        fail("boom")

    result = concur.try_each([1], always_fail)
    assert(not result.ok, "should be error result")

def test_try_exec_failure():
    """Test try_exec wraps error in Result."""
    def always_fail():
        fail("boom")

    result = concur.try_exec(always_fail)
    assert(not result.ok, "should be error result")

def test_try_map_all_fail():
    """Test try_map when all items fail."""
    def always_fail(x):
        fail("all fail")

    result = concur.try_map([1, 2, 3], always_fail)
    assert(not result.ok, "should be error result")

def test_error_propagation():
    """Test error message contains expected text."""
    def custom_error(x):
        fail("custom error message xyz")

    result = concur.try_map([1], custom_error)
    assert(not result.ok, "should fail")
    assert("custom error message xyz" in result.error, "error should contain custom message")
