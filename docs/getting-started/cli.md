---
title: "Use the kite CLI"
description: "Common kite subcommands run against a single hello script"
weight: 5
---

# Use the kite CLI

The `kite` binary exposes its functionality through subcommands. The most common is `run <script>` to execute a `.star` file, but kite also offers a one-line evaluator (`exec`), an interactive REPL (`repl`), a test runner (`test`), a syntax validator (`validate`), and a file-watch loop (`watch`). When the first positional argument resolves to a script path, the `run` subcommand is implicit — so the most frequent case shortens to `kite <script>`.

The commands here each invoke against [`examples/core/hello.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/hello.star) to show the typical shape.

## kite version

```bash
$ kite version --short
v0.1.0
```

## kite run &lt;script&gt;

```bash
$ kite run examples/core/hello.star
Hello from starkite!
Hostname: dev-host.local
User:     alice
Cwd:      /home/alice/projects/starkite
Kernel:   Linux
Time:     2026-01-15T10:00:00Z
Home:     /home/alice
```

## kite &lt;script&gt; (shorthand)

`run` is the implicit subcommand when the first arg is a path:

```bash
$ kite examples/core/hello.star
# same output as `kite run examples/core/hello.star`
```

## kite exec '&lt;code&gt;'

Run a one-liner without a script file:

```bash
$ kite exec 'print("hello from exec")'
hello from exec
```

## kite validate &lt;script&gt;

Parse and type-check without executing:

```bash
$ kite validate examples/core/hello.star
examples/core/hello.star: OK
```

## kite test &lt;test_script&gt;

Run every `def test_*` in a `_test.star` file or directory:

```bash
$ kite test examples/core/hello_test.star
Found 1 test file(s)
============================================================
Tests: 6 passed, 0 failed, 6 total
Time:  15ms
============================================================
```

## kite repl

Interactive REPL with all built-in modules pre-loaded. Requires a TTY; exit with Ctrl-D.

```bash
$ kite repl
>>> print(runtime.platform())
>>> _
```

## kite watch &lt;script&gt;

Re-run a script on every file change. Stop with Ctrl-C.

```bash
$ kite watch examples/core/hello.star
```

## See also

- [CLI reference](../references/cli/index.md) — every subcommand, every flag, every exit code
