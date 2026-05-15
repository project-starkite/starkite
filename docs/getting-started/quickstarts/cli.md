---
title: "Use the kite CLI"
description: "Common kite subcommands run against a single hello script"
weight: 5
---

# Use the kite CLI

Every command below runs against [`examples/core/hello.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/hello.star) and is verified by [`examples/core/cli-quickstart-verify.sh`](https://github.com/project-starkite/starkite/blob/main/examples/core/cli-quickstart-verify.sh) — re-run the script after any CLI change to confirm the docs still match.

## kite version

```bash
$ kite version --short
v0.1.0
```

## kite run &lt;script&gt;

```bash
$ kite run examples/core/hello.star
Hello from starkite!
Hostname: vladimirs-mbp.lan
User:     vladimirvivien
Cwd:      /Users/vladimirvivien/DEV/starkite
Kernel:   Darwin
Time:     2026-05-15T08:52:25-04:00
Home:     /Users/vladimirvivien
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

Interactive REPL with all built-in modules pre-loaded — no scripted output (TTY required). Quit with Ctrl-D.

```bash
$ kite repl
>>> print(runtime.platform())
>>> _
```

## kite watch &lt;script&gt;

Re-run a script on every file change — no scripted output (long-running). Quit with Ctrl-C.

```bash
$ kite watch examples/core/hello.star
```

## See also

- [CLI reference](../../references/cli/index.md) — every subcommand, every flag, every exit code
