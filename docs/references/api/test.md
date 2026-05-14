---
title: "test"
description: "Testing assertions and test control"
weight: 26
---

The `test` module provides assertion functions for writing tests. All functions are also registered as **global aliases**, so you can use them without the `test.` prefix.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `test.assert(condition, msg?, *args)` | `None` | Assert that `condition` is truthy |
| `test.assert_equal(actual, expected, msg?)` | `None` | Assert that `actual == expected` |
| `test.assert_not_equal(actual, unexpected, msg?)` | `None` | Assert that `actual != unexpected` |
| `test.assert_contains(haystack, needle, msg?)` | `None` | Assert that `haystack` contains `needle` |
| `test.assert_true(value, msg?)` | `None` | Assert that `value` is `True` |
| `test.assert_false(value, msg?)` | `None` | Assert that `value` is `False` |
| `test.skip(reason?)` | `None` | Skip the current test with an optional reason |
| `test.fail(msg)` | `None` | Fail the current test with a message |

## Global Aliases

All functions are available as top-level globals:

```python
# These are identical
test.assert_equal(1 + 1, 2)
assert_equal(1 + 1, 2)
```

## Examples

### Basic assertions

```python
def test_math():
    assert_equal(1 + 1, 2)
    assert_not_equal(1 + 1, 3)
    assert_true(10 > 5)
    assert_false(10 < 5)
```

### Custom messages

```python
def test_deployment():
    result = exec("kubectl get pods -l app=web -o json")
    assert(result.ok, "kubectl command failed: %s", result.stderr)

    data = json.decode(result.stdout)
    assert_equal(len(data["items"]), 3, "expected 3 pods")
```

### Contains assertion

```python
def test_output():
    result = exec("kite version")
    assert_contains(result.stdout, "kite")
```

### Skipping tests

```python
def test_linux_only():
    if runtime.platform() != "linux":
        skip("linux only")
    # linux-specific test logic
    result = exec("systemctl status nginx")
    assert(result.ok)
```

### Failing explicitly

```python
def test_not_implemented():
    fail("TODO: implement this test")
```

### Running tests

Tests are `.star` files containing functions prefixed with `test_`. Run them with:

```bash
kite test tests/
kite test tests/deploy_test.star
```
