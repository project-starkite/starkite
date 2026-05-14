---
title: "What is Starlark?"
description: "The language behind starkite scripts"
weight: 2
---

Starkite scripts are written in [Starlark](https://github.com/bazelbuild/starlark), a small, deterministic, Python-derived language originally developed at Google for the [Bazel](https://bazel.build) build system. If you've written Python, Starlark will feel immediately familiar — but it deliberately removes several features in exchange for being safer to embed and easier to reason about.

## A small, familiar subset of Python

A Starlark script reads almost identically to Python:

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

You get the things you'd expect: functions, lambdas, `if`/`for`, list/dict/set comprehensions, string/list/dict/tuple types, slicing, exceptions via `fail()`, and a familiar standard library of `len()`, `range()`, `sorted()`, `enumerate()`, and friends.

## What's intentionally absent

Starlark is **not** Python. It removes constructs that make programs hard to predict or sandbox:

- **No `while` loops.** Only bounded `for`-loops over a collection.
- **No recursion.** Functions cannot call themselves (directly or indirectly).
- **No mutable globals after initialization.** Top-level bindings are frozen once the script's module finishes loading.
- **No classes or inheritance.** Use dicts, tuples, and functions.
- **No I/O in the language itself.** Filesystem, network, processes — everything privileged comes from modules the runtime provides.
- **No exceptions you can catch.** Errors halt the script unless the function you called offers a `try_` variant.

These omissions are the *point*. A Starlark program is guaranteed to terminate, never silently mutates shared state, and runs identically every time given identical inputs — properties that make it ideal for build systems, configuration, and ops automation.

## Why this fits ops automation

Operations scripts are unforgiving: they run on production, they're triggered by humans under stress, and they're rarely re-executed in a clean environment. Starkite leans on Starlark's guarantees to keep scripts predictable:

- **Determinism.** A `deploy.star` that succeeds on your laptop will execute the same way in CI and on the prod jumpbox.
- **Bounded execution.** No infinite loops, no runaway recursion. Scripts finish or fail; they don't hang.
- **No hidden state.** Everything a script touches goes through a typed module call you can audit and a [permission rule](../fundamentals/security/permissions.md) you can deny.

## Where to go next

- [Getting Started](index.md) walks you through writing your first script.
- [Language](../fundamentals/language.md) covers starkite-specific extensions: variable injection and the `try_` error pattern.
- [Modules](../fundamentals/modules.md) explains how built-in capabilities are organized and loaded.
- The [Starlark spec](https://github.com/bazelbuild/starlark/blob/master/spec.md) is the authoritative reference for the language itself.
