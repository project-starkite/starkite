---
title: "Run a Starkite script"
description: "Write and run your first Starkite script - hello.star"
weight: 1
---

# Hello.star: your first script

A Starkite script is a Starlark source file executed by the `kite` runtime. This guide walks you through writing a basic script, executing it as a command-line tool, and using built-in system modules to read environment and system metrics.

Starkite supports two script execution patterns:

*   **Top-level Execution**: Statements are executed sequentially from the top of the file, suitable for simple, linear scripts.
*   **Main Entry Point**: The runtime automatically invokes a defined `main()` function after executing top-level declarations. This is standard for structured scripts and modules.

## Writing a simple script

Create a file named `hello.star` with a top-level print statement:

```python
# hello.star
print("Hello from starkite!")
```

Execute the script using the `kite run` command:

```bash
$ kite run ./hello.star
Hello from starkite!
```

Execution is direct: there is no compilation step, entry point declaration, or package import required.

## Executing scripts directly

On Unix-like systems (Linux and macOS), you can run the script directly as an executable by adding a shebang line pointing to `kite`:

```python
#!/usr/bin/env kite
# hello.star
print("Hello from starkite!")
```

Mark the file as executable using `chmod`, then run it directly by its path:

```bash
$ chmod +x ./hello.star
$ ./hello.star
Hello from starkite!
```

Adding a shebang allows the script to behave like a standard command-line utility.

## Using main() and built-in modules

For structured automation, group your script logic inside a `main()` function and leverage Starkite's pre-loaded standard library. You do not need to write import or `load()` statements to access these library functions.

Update `hello.star` to define a `main()` entry point and query host system details:

```python
#!/usr/bin/env kite
# hello.star

def main():
    print("Hello from starkite!")

    printf("Hostname: %s\n", hostname())
    printf("User:     %s\n", username())
    printf("Cwd:      %s\n", cwd())

    # Execute host commands using the os module
    uname = os.exec("uname -s").strip()
    printf("Kernel:   %s\n", uname)

    # Format the current time and check environments
    printf("Time:     %s\n", time.format(time.now(), time.RFC3339))
    printf("Home:     %s\n", env("HOME", "/tmp"))
```

In this script:

*   `main()` is automatically invoked by the `kite` runtime.
*   `hostname()`, `username()`, and `cwd()` return the host execution context.
*   `os.exec("uname -s")` executes an external command on the host.
*   `time.now()` and `time.format()` manage timestamps.
*   `env()` queries environment variables, returning a fallback value (`/tmp`) if the key is missing.

Run the updated script:

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

For a complete list of built-in modules and function signatures, see the [Starkite API Reference](../references/api/index.md).

Refer to the full example in the repository: [`examples/core/hello.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/hello.star).
