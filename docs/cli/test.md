---
title: "kite test"
description: "Run starkite tests"
weight: 3
---

Run test scripts. Test files should end with `_test.star` and contain functions prefixed with `test_`.

## Usage

```bash
kite test <path> [flags]
```

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-v, --verbose` | Verbose test output | `false` |
| `-p, --parallel N` | Parallel test file runners | `1` |
| `--run pattern` | Run only tests matching substring | |

## Test Functions

```python
# math_test.star

def setup():
    """Runs before each test (optional)."""
    pass

def teardown():
    """Runs after each test (optional)."""
    pass

def test_addition():
    assert_equal(1 + 1, 2)

def test_strings():
    assert_contains("hello world", "hello")

def test_conditional():
    if runtime.platform() != "linux":
        skip("linux only")
    assert_true(exists("/etc/hosts"))
```

## Built-in Assertions

| Function | Description |
|----------|-------------|
| `assert(condition, msg?)` | Fail if condition is falsy |
| `assert_equal(actual, expected, msg?)` | Deep equality check |
| `assert_not_equal(actual, unexpected, msg?)` | Not equal check |
| `assert_contains(haystack, needle, msg?)` | Containment check (strings, lists, dicts) |
| `assert_true(value, msg?)` | Truthiness check |
| `assert_false(value, msg?)` | Falsiness check |
| `skip(reason?)` | Skip the current test |
| `fail(msg)` | Unconditionally fail |

## Examples

```bash
kite test ./tests/                     # Run all *_test.star
kite test ./tests/math_test.star       # Run single file
kite test ./tests/ --verbose           # Verbose output
kite test ./tests/ --run string        # Filter by name
kite test ./tests/ --parallel 4        # Parallel execution
```
