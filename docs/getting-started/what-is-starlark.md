---
title: "What is Starlark?"
description: "The language behind starkite scripts"
weight: 2
---

Starkite scripts are written in [Starlark](https://github.com/bazelbuild/starlark), a small, deterministic, Python-derived language used by the [Bazel](https://bazel.build) build system. Starlark is a Python subset that removes constructs incompatible with safe embedding and predictable execution.

## A Python subset

```python
name    = var_str("env", "dev")
servers = ["web-1", "web-2", "web-3"]

def deploy(host):
    print("deploying to", host)
    return os.exec("ssh " + host + " systemctl restart app")

for host in servers:
    result = deploy(host)
    if result.exit_code != 0:
        fail("deploy failed on " + host)
```

Available: functions, lambdas, `if`/`for`, list/dict/set comprehensions, string/list/dict/tuple types, slicing, exceptions via `fail()`, and a standard library of `len()`, `range()`, `sorted()`, `enumerate()`, etc.

## Removed constructs

- **No `while` loops.** Only bounded `for`-loops over a collection.
- **No recursion.** Functions cannot call themselves, directly or indirectly.
- **No mutable globals after initialization.** Top-level bindings are frozen once the script's module finishes loading.
- **No classes or inheritance.** Use dicts, tuples, and functions.
- **No I/O in the language itself.** Filesystem, network, and process operations come from modules the runtime provides.
- **No catchable exceptions.** Errors halt the script unless the function offers a `try_` variant.

## Guarantees this provides

- **Determinism.** Identical inputs produce identical execution.
- **Bounded execution.** No infinite loops or runaway recursion. Scripts terminate or fail.
- **No hidden state.** Every privileged operation goes through a typed module call subject to a [permission rule](../fundamentals/security/permissions.md).

## Where to go next

- [Getting Started](index.md) walks you through writing your first script.
- [Language](../fundamentals/language.md) covers starkite-specific extensions: variable injection and the `try_` error pattern.
- [Modules](../fundamentals/modules.md) explains how built-in capabilities are organized and loaded.
- The [Starlark spec](https://github.com/bazelbuild/starlark/blob/master/spec.md) is the authoritative reference for the language itself.
