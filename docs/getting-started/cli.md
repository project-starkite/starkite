---
title: "Use the kite CLI"
description: "Introduction to Starkite CLI commands"
weight: 5
---

# Use the kite CLI

The `kite` command-line interface is the primary utility for executing, validating, testing, and debugging Starkite scripts. It provides dedicated subcommands for inline code evaluation, syntax validation, test harness execution, and hot-reload file watching. The walkthrough below demonstrates these common CLI operations using the local `./hello.star` script created in the previous section.

## kite version

To output the version of the installed `kite` binary:

```bash
$ kite version --short
v0.1.0
```

Without the `--short` flag, this command outputs complete build details, including commit hashes, build timestamps, the compiler version, and the target architecture.

## kite run

The `run` subcommand executes a Starlark script file end-to-end:

```bash
$ kite run ./hello.star
Hello from starkite!
Hostname: dev-host.local
User:     alice
Cwd:      /home/alice/projects/starkite
Kernel:   Linux
Time:     2026-01-15T10:00:00Z
Home:     /home/alice
```

## kite (shorthand)

For convenience, if the first positional argument passed to `kite` is a script file path, the `run` subcommand is implicit. This makes running the bare file name equivalent to using the explicit `run` subcommand:

```bash
$ kite ./hello.star
Hello from starkite!
Hostname: dev-host.local
User:     alice
Cwd:      /home/alice/projects/starkite
Kernel:   Linux
Time:     2026-01-15T10:00:00Z
Home:     /home/alice
```

This shorthand is what enables shebang (`#!`) direct execution. When you execute a script file via `./hello.star`, the operating system invokes `kite ./hello.star` under the hood. Because the `run` subcommand is assumed automatically, the runtime executes the script directly instead of failing due to a missing subcommand.

For example, given a script with the `kite` shebang at the top:

```python
#!/usr/bin/env kite
# hello.star
print("Hello from starkite!")
# ...
```

Executing it directly from the shell:

```bash
# Mark the script as executable (if not already done)
$ chmod +x ./hello.star

# Run the script directly using the shebang shorthand
$ ./hello.star
Hello from starkite!
Hostname: dev-host.local
User:     alice
Cwd:      /home/alice/projects/starkite
Kernel:   Linux
Time:     2026-01-15T10:00:00Z
Home:     /home/alice
```


## kite exec

The `exec` subcommand executes Starlark code passed directly as an inline string argument. This is useful for evaluating expressions, testing snippets, or running inline automation checks:

```bash
$ kite exec 'print("Hello from exec")'
Hello from exec
```

For advanced shell pipelines, JSON/YAML processing, network queries, and script assertions, see the [Shell Integration Guide](../fundamentals/shell-integration.md).

## kite validate

To check a script's syntax without executing any of its logic, use `validate`. This command parses the script and reports any syntax or structural errors:

```bash
$ kite validate ./hello.star
./hello.star: OK
```

## kite test

To run test suites written in Starlark, point `test` at a test script file (typically suffixed with `_test.star`) or a directory containing them. The runner automatically executes any functions named with the `test_` prefix:

```bash
# Example running tests against a test file
$ kite test ./hello_test.star
Found 1 test file(s)
============================================================
Tests: 6 passed, 0 failed, 6 total
Time:  15ms
============================================================
```

For more details on writing test cases and using assertions, see [Testing](../fundamentals/testing.md).

## kite repl

To interactively evaluate Starlark code, launch the read-eval-print loop (REPL) session:

```bash
$ kite repl
>>> print(runtime.platform())
darwin
>>> 
```

## kite watch

To speed up development, `watch` monitors a script file and automatically re-executes it every time the file is modified:

```bash
$ kite watch ./hello.star
```

For a complete list of CLI subcommands, flags, and environment variables, see the [CLI Reference](../references/cli/index.md).
