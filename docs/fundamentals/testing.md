---
title: "Testing"
description: "Write and run tests with the built-in test runner"
weight: 50
---

# Testing

Starkite includes a built-in test runner that executes Starlark-based script tests. Functions prefixed with `test_` are automatically discovered, run, and reported by the harness. Tests execute under the same runtime engine, permission profiles, and sandbox settings as production scripts.

## Writing tests

Any top-level function starting with `test_` is treated as a test case. The runner executes these functions sequentially. To verify expectations, use global assertion functions:

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

The following assertions are available globally (or via the explicit `test.` namespace):

| Function | Checks |
|----------|--------|
| `assert(cond, msg?, *args)` | `cond` is truthy |
| `assert_equal(actual, expected, msg?, *args)` | `actual == expected` |
| `assert_not_equal(actual, unexpected, msg?, *args)` | `actual != unexpected` |
| `assert_contains(haystack, needle, msg?, *args)` | `haystack` contains `needle` |
| `assert_true(value, msg?, *args)` | `value` is `True` |
| `assert_false(value, msg?, *args)` | `value` is `False` |
| `skip(reason?)` | Skips the current test |
| `fail(msg)` | Fails the current test immediately |

Assertions accept an optional, printf-style format string and arguments for custom failure messages:

```python
def test_deployment():
    result = try_exec("kubectl get pods -l app=web -o json")
    assert(result.ok, "kubectl failed: %s", result.stderr)
```

When this assertion fails, the test report prints the actual `kubectl` error output. For complete signatures, see the [test API reference](../references/api/test.md).

### Setup and teardown

To avoid repeating preparation and cleanup logic, define `setup()` and `teardown()` functions. The runner executes `setup()` before each test and `teardown()` after each test, even if a test case fails:

```python
def setup():
    exec("kubectl create namespace test-ns")

def teardown():
    exec("kubectl delete namespace test-ns")

def test_deploy():
    result = try_exec("kubectl apply -f manifest.yaml -n test-ns")
    assert(result.ok, "apply failed: %s", result.stderr)
```

## Running tests

The `kite test` command accepts a single file or a directory. When given a directory, it executes all files ending in `_test.star`:

```bash
kite test math_test.star      # Execute a single file
kite test ./tests/            # Execute all *_test.star files in a directory
```

Use optional flags to configure the test execution:

```bash
kite test ./tests/ --verbose       # Display assertion details
kite test ./tests/ --run sql       # Run only tests matching a name pattern
kite test ./tests/ --parallel 4    # Run up to four files concurrently
```

> [!NOTE]
> Unlike standard executions, the test runner does not invoke `main()`. It executes only the discovered `test_*` functions.

## Permissions and tests

Tests run under the same [permission](security/permission.md) rules as standard executions. Without explicit flags, tests run under the `deny-all` profile. If your tests perform I/O, network requests, or shell commands, pass an appropriate permission profile:

```bash
kite test ./tests/ --permissions=allow-local
```

To ensure test reliability, run your test suite under the same permission profile the script uses in production.

## Sandbox isolation

On Linux systems, you can isolate test execution. The `kite test --sandbox` command runs **each test file in its own separate sandbox process**, preventing tests from mutating host files or leaking state to other tests. 

For maximum security, combine sandbox isolation with explicit API permissions:

```bash
kite test ./tests/ --sandbox=opaque --permissions=allow-fs
```

For more details on sandbox isolation, see the [Sandbox Guide](security/sandbox.md).
