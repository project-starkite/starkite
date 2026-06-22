---
title: "Use the kite CLI"
description: "Introduction to Starkite CLI commands"
weight: 5
---

# Use the kite CLI

The `kite` binary is how you drive Starkite from the shell: you point it at a `.star` file and it runs your automation, but the same binary also evaluates a one-liner, opens an interactive session, runs a test suite, checks a script for errors, and re-runs a script as you edit it. All of that hangs off subcommands, and the one you reach for most is `run <script>`. Because running a file is the common case, kite makes it the default — when the first positional argument resolves to a script path, the `run` subcommand is implicit, so `kite <script>` and `kite run <script>` do the same thing.

The walkthrough below drives each command against [`examples/core/hello.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/hello.star) so the shape of every invocation is concrete.

## kite version

Before running anything, confirm which build you have. `kite version` prints the full report — edition, commit, build time, runtime — and `--short` narrows it to just the version string, which is the form to use in scripts and CI checks:

```bash
$ kite version --short
v0.1.0
```

## kite run &lt;script&gt;

`run` is the workhorse: hand it a script and it executes the file end to end, printing whatever the script writes:

```bash
$ kite run ./examples/core/hello.star
Hello from starkite!
Hostname: dev-host.local
User:     alice
Cwd:      /home/alice/projects/starkite
Kernel:   Linux
Time:     2026-01-15T10:00:00Z
Home:     /home/alice
```

The output above is host-specific — your hostname, user, paths, and time will differ.

## kite &lt;script&gt; (shorthand)

Since `run` is what you do most, you rarely have to type it. When the first argument is a path, kite supplies `run` for you, so the previous command collapses to the bare script:

```bash
$ kite ./examples/core/hello.star
# same output as `kite run ./examples/core/hello.star`
```

## kite exec '&lt;code&gt;'

When the work is too small to warrant a file, skip the script entirely. `kite exec` takes Starlark source on the command line and runs it directly, which is the quickest way to test a snippet or compute a one-off value:

```bash
$ kite exec 'print("hello from exec")'
hello from exec
```

## kite validate &lt;script&gt;

To catch errors before a script ever runs — in CI, a pre-commit hook, or an editor — use `validate`. It parses the script without executing it and reports whether the file is well-formed, so a broken script fails the check rather than failing partway through a real run:

```bash
$ kite validate examples/core/hello.star
examples/core/hello.star: OK
```

## kite test &lt;test_script&gt;

For scripts you mean to keep working, `test` runs the suite. Point it at a `_test.star` file or a directory and it discovers every `def test_*`, runs each one, and reports the tally:

```bash
$ kite test examples/core/hello_test.star
Found 1 test file(s)
============================================================
Tests: 6 passed, 0 failed, 6 total
Time:  15ms
============================================================
```

See [Testing](../fundamentals/testing.md) for writing tests, assertions, and the run flags.

## kite repl

When you want to explore interactively rather than commit to a file, `repl` opens a session with every built-in module pre-loaded, so you can call them and inspect results line by line. It requires a TTY; exit with Ctrl-D:

```bash
$ kite repl
>>> print(runtime.platform())
>>> _
```

## kite watch &lt;script&gt;

To tighten the edit-run loop, `watch` re-runs a script every time its file changes, so you see the effect of each save without re-invoking kite by hand. Stop the loop with Ctrl-C:

```bash
$ kite watch ./examples/core/hello.star
```

## See also

- [CLI reference](../references/cli/index.md) — every subcommand, every flag, every exit code
