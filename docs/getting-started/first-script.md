---
title: "Run a Starkite script"
description: "Write and run your first Starkite script - hello.star"
weight: 1
---

# Hello.star: your first script

A Starkite script is an ordinary Starlark file that you run with the `kite` command. By the end of this page you will have written one, run it two different ways, and extended it to read live information from the machine it runs on — enough to see how a script goes from a file on disk to a working program.

Start with the smallest script that does something visible. A script lives in a file with the `.star` extension, so create `hello.star` with a single line:

```python
# hello.star

print("Hello from starkite!")
```

To run it, hand the file to `kite run`:

```
$ kite run ./hello.star

Hello from starkite!
```

The `print` call wrote its argument to standard output, and `kite` did the rest — there was no build step, no entry point to declare, and no module to import. That is the whole loop: write a `.star` file, run it with `kite run`.

## Run the script as an executable

Typing `kite run` each time works, but on Linux and macOS you can make the script run itself. Add a hashbang (`#!`) line at the top that points at `kite`, and the operating system will invoke `kite` for you when you execute the file directly:

```python
#!/usr/bin/env kite
# hello.star

print("Hello from starkite!")
```

The hashbang only takes effect once the file is marked executable, so set the executable bit and then run the file by its path:

```
$ chmod +x hello.star
$ ./hello.star

Hello from starkite!
```

The output is identical — the difference is purely in how the script is launched. With the hashbang in place, `hello.star` behaves like any other command-line tool, which is what you want once a script graduates from a quick experiment to something you call by name.

## Starkite comes with everything

A script this small does not need much, but real automation reaches for the filesystem, the system clock, the environment, and shells out to other programs. Starkite ships a standard set of library modules for exactly this, loaded into every script without an import, so you can call them as though they were built into the language. Extend the script to report on the machine it is running on:

```python
#!/usr/bin/env kite
# hello.star

print("Hello from starkite!")

printf("Hostname: %s\n", hostname())
printf("User:     %s\n", username())
printf("Cwd:      %s\n", cwd())

uname = os.exec("uname -s").strip()
printf("Kernel:   %s\n", uname)

printf("Time:     %s\n", time.format(time.now(), time.RFC3339))
printf("Home:     %s\n", env("HOME", "/tmp"))
```

Each line pulls a fact from a different corner of the system. `hostname()`, `username()`, and `cwd()` report where and as whom the script is running; `os.exec("uname -s")` runs a real command and `.strip()` trims the trailing newline off its output; `time.now()` and `time.format(...)` produce a timestamp in RFC 3339 form; and `env("HOME", "/tmp")` reads an environment variable, falling back to `/tmp` if it is unset. None of these required a `load()` statement — they arrive with the runtime.

Run it the same way as before, and the values reflect your own machine:

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

Your output will differ — the hostname, user, paths, and time are specific to the host that ran the script.

You have now written a Starkite script, run it both through `kite run` and as a self-executing file, and used the built-in modules to read from the system. For the full set of modules available to a script, see [References > API](../references/api/index.md).

*See example* [`examples/core/hello.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/hello.star).
