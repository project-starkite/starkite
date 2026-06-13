---
title: "Testing"
description: "Write and run tests with the built-in test runner"
weight: 50
---

# Testing

Starkite is shipped with a built-in test runner that turns an ordinary script into a test suite. Write functions whose names begin with `test_`, and the runner discovers them, runs each one, and reports which passed and which failed — there is no framework to import and no harness to wire up. A test runs through the same engine, assertions, permission model, and sandbox you use to run a script, so it exercises your automation the same way a real run does.

You point the runner at a file or a directory with `kite test`, and it takes care of the rest.

## Writing tests

A test is a function whose name starts with `test_`. When the runner loads a file it scans the top-level definitions, collects every `test_*` function, and calls them one at a time. Nothing else is required to register a test — defining the function is enough.

Inside a test you state what you expect to be true, and you do that with **assertions**: built-in functions that pass silently when a condition holds and fail the test, with a message, when it does not. Assertions are available as globals, so you call them directly without a module prefix:

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

Each assertion checks one kind of condition. The table below lists the built-ins; every one is also reachable under the `test.` namespace (`test.assert_equal`, and so on) if you prefer an explicit prefix.

| Function | Checks |
|----------|--------|
| `assert(cond, msg?, *args)` | `cond` is truthy |
| `assert_equal(actual, expected, msg?, *args)` | `actual == expected` |
| `assert_not_equal(actual, unexpected, msg?, *args)` | `actual != unexpected` |
| `assert_contains(haystack, needle, msg?, *args)` | `haystack` contains `needle` |
| `assert_true(value, msg?, *args)` | `value` is `True` |
| `assert_false(value, msg?, *args)` | `value` is `False` |
| `skip(reason?)` | skips the current test |
| `fail(msg)` | fails the current test immediately |

Every assertion takes an optional message as its trailing argument, and that message is printf-style — a format string followed by the values to interpolate. Reach for it whenever a bare pass or fail would not tell you what went wrong:

```python
def test_deployment():
    result = exec("kubectl get pods -l app=web -o json")
    assert(result.ok, "kubectl failed: %s", result.stderr)
```

When this assertion fails, the report shows the actual `kubectl` error rather than a generic "assertion failed". See the [test API reference](../references/api/test.md) for the full signatures.

### Setup and teardown

Most tests need the same starting conditions — a temporary namespace, a seeded database, a scratch directory. Rather than repeat that work in every `test_*` function, define a `setup()` and a `teardown()`. The runner calls `setup()` before each test and `teardown()` after each test, so every test starts from a known state and cleans up after itself even when it fails:

```python
def setup():
    exec("kubectl create namespace test-ns")

def teardown():
    exec("kubectl delete namespace test-ns")

def test_deploy():
    result = exec("kubectl apply -f manifest.yaml -n test-ns")
    assert(result.ok, "apply failed: %s", result.stderr)
```

## Running tests

The runner accepts either a single file or a directory. Hand it one file and it runs that file as given, whatever its name; hand it a directory and it walks the tree and runs every file ending in `_test.star`:

```bash
kite test math_test.star      # one file, run as given
kite test ./tests/            # every *_test.star under a directory
```

A few flags shape a run. Add `--verbose` (`-v`) to watch each assertion as it executes instead of a final summary; narrow a large suite with `--run`, which keeps only tests whose name contains the given substring; and shorten a slow suite with `--parallel` (`-p`), which runs that many test files at once:

```bash
kite test ./tests/ --verbose       # per-assertion detail
kite test ./tests/ --run sql       # only tests whose name contains "sql"
kite test ./tests/ --parallel 4    # run four files concurrently
```

One behavior differs from a normal run: in test mode the runner never auto-invokes `main()`. It calls your `test_*` functions directly, so a `main()` you wrote as the script's normal entry point stays out of the way.

## Permissions and tests

A test runs under the same [permission](security/permission.md) model as the script it covers, and that includes the default. With no profile, tests run under `deny-all` — pure computation is allowed, but the filesystem, the network, processes, and the environment are not. A test that reads a file, calls an API, or shells out therefore needs a profile that grants it:

```bash
kite test ./tests/ --permissions=allow-local
```

Run your tests under the profile the script will use in production. A suite that passes under a tighter profile than the real run gives false confidence, and one that needs a looser profile is a sign the test is reaching for something the script should not.

## Isolating tests under the sandbox

On Linux you can contain the tests themselves. `kite test --sandbox` runs **each test file in its own sandbox** — a separate process under gVisor's OS-level isolation — so a test that writes files or opens sockets cannot touch the host, and two files cannot leak state into each other through `/tmp` or a shared listener. The sandbox is Linux-only; on macOS or Windows, requesting it returns an error. See [Sandbox](security/sandbox.md) for the isolation model and the available profiles.

For an untrusted or unfamiliar suite, pair the two: `--sandbox` confines what the test process can reach on the host, and `--permissions` restricts which Starkite APIs the test may call. Together they let you run code you do not fully trust and still trust the result.
