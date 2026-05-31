---
title: "Testing"
description: "Write and run tests with the built-in test runner"
weight: 50
---

# Testing

Starkite ships a built-in test runner. Test functions are named `test_*` and use assertion built-ins; `kite test` discovers and runs them.

## Writing tests

A test is a function whose name starts with `test_`. Assertion functions are available as globals (no `test.` prefix needed):

```python
# math_test.star
def test_addition():
    assert_equal(1 + 1, 2)
    assert_true(10 > 5)

def test_strings():
    assert_contains("hello world", "world")
    assert_not_equal("a", "b")
```

### Assertions

| Function | Description |
|----------|-------------|
| `assert(cond, msg?, *args)` | Assert `cond` is truthy |
| `assert_equal(actual, expected, msg?)` | Assert `actual == expected` |
| `assert_not_equal(actual, unexpected, msg?)` | Assert `actual != unexpected` |
| `assert_contains(haystack, needle, msg?)` | Assert `haystack` contains `needle` |
| `assert_true(value, msg?)` | Assert `value` is `True` |
| `assert_false(value, msg?)` | Assert `value` is `False` |
| `skip(reason?)` | Skip the current test |
| `fail(msg)` | Fail the current test |

`assert` takes a printf-style message for context on failure:

```python
def test_deployment():
    result = exec("kubectl get pods -l app=web -o json")
    assert(result.ok, "kubectl failed: %s", result.stderr)
```

See the [test API reference](../references/api/test.md) for the full list.

## Running tests

```bash
kite test math_test.star      # a single file
kite test ./tests/            # every *_test.star under a directory
kite test ./tests/ --verbose  # per-assertion detail
```

`main()` auto-invocation does not apply in test mode — the runner discovers and calls `test_*` functions directly.

## Isolating tests under the sandbox

On Linux, `kite test --sandbox` runs each test file under OS-level isolation, so a test that touches the filesystem or network is contained. See [Sandbox](security/sandbox.md) for the isolation model and profiles.
