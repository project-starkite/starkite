---
title: "Launching processes"
description: "Execute local commands and manage sub-processes in Starkite"
weight: 20
---

# Launching processes

Starkite separates direct, shell-free binary execution from shell-wrapped execution. This approach enforces strong security boundaries while maintaining flexibility for developers.

The `os` module provides four functions to execute commands:

*   **`os.exec(cmd, args=[])`**
    Executes a binary directly and shell-free. If the command returns a non-zero exit code, it halts execution and raises a Starlark-level error.

*   **`os.try_exec(cmd, args=[])`**
    Executes a binary directly and shell-free, returning an `ExecResult` for programmatic error handling.

*   **`os.shell(cmd)`**
    Runs a command string inside a shell wrapper (default: `/bin/sh -c`), allowing pipes, redirections, and environment expansion. It halts execution and raises a Starlark-level error on non-zero exit codes.

*   **`os.try_shell(cmd)`**
    Runs a command string inside a shell wrapper, returning an `ExecResult` for programmatic error handling.

---

## Direct Process Execution (`os.exec` & `os.try_exec`)

By default, `os.exec()` and `os.try_exec()` execute target binaries directly and shell-free. This direct execution prevents common shell injection vulnerabilities.

To pass arguments to the binary, specify them as a list/tuple of strings in the second argument:

```python
def check_os():
    # Executes 'uname' directly with argument '-a'
    os_info = os.exec("uname", ["-a"])
    print("System OS:", os_info.strip())
```

Alternatively, if only a single string is passed, Starkite automatically splits the string by whitespace to resolve the binary and its arguments:

```python
def check_os_single_string():
    # Automatically splits by whitespace: runs 'uname' with '-a'
    os_info = os.exec("uname -a")
    print("System OS:", os_info.strip())
```

### Permissions

Direct process execution requires the **`allow-all`** permission profile (or a custom profile explicitly granting `os.exec` permissions):

```bash
kite run ./script.star --permissions allow-all
```

---

## Shell-Wrapped Execution (`os.shell` & `os.try_shell`)

For tasks requiring shell features such as pipes (`|`), redirections (`>`, `<`), environment variable expansion (e.g., `$VAR`), command substitution, or filename globbing, use `os.shell()` or `os.try_shell()`. These run the command string inside a shell wrapper (default: `/bin/sh -c` on Unix/Linux systems):

```python
def check_disk_space():
    # Shell wrapper allows pipes and redirections
    disk_info = os.shell("df -h / | tail -1")
    print("Disk Info:", disk_info.strip())
```

### Permissions

Because shell execution allows powerful shell features (like pipes and redirection) that present higher security risks, it is blocked under the default `allow-all` profile. Shell-wrapped execution requires the **`allow-all-shell`** permission profile:

```bash
kite run ./script.star --permissions allow-all-shell
```

---

## Programmatic Error Handling (`os.try_exec` & `os.try_shell`)

Use `os.try_exec()` or `os.try_shell()` when you need to handle exit status codes programmatically. They never raise a Starlark error on command failure; instead, they return an `ExecResult` object.

```python
def check_disk_space_safe():
    # Safe to handle failure programmatically
    disk = os.try_shell(
        "df -h / | tail -1",
        timeout = "5s",
        cwd = "/tmp",
        env = {"LANG": "C"},
    )
    
    if disk.ok:
        fields = disk.stdout.split()
        printf("Disk Available: %s (Used: %s)\n", fields[3], fields[4])
    else:
        printf("Warning: Disk check failed with code %d\n", disk.code)
```

---

## Execution Options

All process execution functions accept the following optional keyword arguments:

| Option | Type | Default | Purpose |
|--------|------|---------|---------|
| `shell` | `string` | `"/bin/sh"` | Shell binary used to execute the command string (applicable to `os.shell` and `os.try_shell` only). |
| `cwd` | `string` | `""` | Working directory in which to run the sub-process. |
| `env` | `dict` | `None` | Environment variable overrides (mapping string to string) for the command execution context. |
| `timeout` | `string` | `"60s"` | Time limit for execution (e.g., `"10s"`, `"5m"`). The process is killed if the timeout is exceeded. |
| `userid` | `string` \| `int` | `None` | User identity under which to run the process (POSIX only). See details below. |
| `groupid` | `string` \| `int` | `None` | Group identity under which to run the process (POSIX only). See details below. |
| `input` | `string` \| `bytes` \| `io.reader` | `None` | Data or read stream to write to the process standard input. |
| `output` | `io.writer` | `None` | Write stream to redirect the process standard output to. |

---

## Handling Results

`os.try_exec()` and `os.try_shell()` return an `ExecResult` struct containing the following attributes:

*   **`.ok`** (`bool`): `True` if the command exited with code `0` and no internal errors occurred.
*   **`.code`** (`int`): The integer process exit code returned by the command.
*   **`.stdout`** (`string`): The standard output stream captured from the command.
*   **`.stderr`** (`string`): The standard error stream captured from the command.
*   **`.error`** (`string`): A combined error message if the command failed or timed out.

---

## User and Group Execution

Local command execution natively supports running sub-processes under specified user and group identities (UID/GID) in POSIX environments (Linux, macOS).

Use the `userid` and `groupid` optional keyword arguments to configure the OS credentials of the spawned process:

*   **`userid`** (`string` | `int`): The username or numeric User ID (UID) of the target user.
*   **`groupid`** (`string` | `int`): The group name or numeric Group ID (GID) of the target group.

### Examples

#### Running as a Specific User (Username)

To run a command as a specific user, pass the username to `userid`:

```python
def query_database():
    # Runs the psql command directly as the postgres user
    result = os.try_exec("psql", ["-c", "SELECT version();"], userid="postgres")
    if result.ok:
        print("Database Version:", result.stdout.strip())
```

#### Running with Numeric IDs

To run a command using specific numeric UID and GID:

```python
def run_unprivileged_task():
    # Runs the command as UID 65534 (nobody) and GID 65534 (nogroup)
    result = os.exec("id", userid=65534, groupid=65534)
    print("Identity:", result.strip())
```

### Required Privileges

Changing process credentials (setuid/setgid) requires the parent `kite` process to have sufficient operating system privileges (typically running as `root` or having `CAP_SETUID`/`CAP_SETGID` capabilities). If `kite` is run under a standard non-privileged user, the OS kernel will reject the credential switch, and `os.exec()` will return a standard OS permission error (e.g., `operation not permitted`).

---

## Streaming Input and Output

Subprocess execution integrates with the unified streaming contract via the `input` and `output` keyword arguments. This allows piping data directly into a process's standard input and redirecting its standard output.

### Streamable Inputs

The `input` argument accepts:
*   `string`: Passed directly as standard input to the command.
*   `bytes`: Passed directly as standard input to the command.
*   `io.reader`: An active read stream (such as a stream returned by `fs.path.get_reader()` or an HTTP client response stream). The stream is copied to the subprocess standard input.

Once the command terminates (or times out), the input stream is automatically closed by the runtime.

### Streamable Outputs

The `output` argument accepts:
*   `io.writer`: An active write stream (such as a stream returned by `fs.path.get_writer()`). The subprocess standard output is written directly to the stream.

Once the command terminates (or times out), the output stream is automatically flushed and closed by the runtime.

### Examples

#### Piping a File to a Process

To pipe a file's content directly into the standard input of a process:

```python
def count_lines():
    p = fs.path("data.txt")
    p.write_text("line 1\nline 2\nline 3\n")
    
    # Streams file data directly to the stdin of 'wc -l'
    reader = p.get_reader()
    result = os.exec("wc -l", input=reader)
    print("Line count:", result.strip())
```

#### Piping Process Output to a File

To redirect a process's standard output directly to a file:

```python
def save_system_info():
    out_file = fs.path("sysinfo.txt")
    
    # Redirects stdout of 'uname -a' to the file writer
    writer = out_file.get_writer()
    os.exec("uname -a", output=writer)
    
    print("Saved content:", out_file.read_text().strip())
```

#### Multi-stage Pipeline (File-to-File via Subprocess)

To stream from an input file, process it with a command, and write the output directly to another file:

```python
def process_pipeline():
    in_file = fs.path("input.log")
    out_file = fs.path("output.log")
    
    in_file.write_text("debug log entry\nerror log entry\ninfo log entry\n")
    
    r = in_file.get_reader()
    w = out_file.get_writer()
    
    # Pipes input.log into 'grep error' and writes the output directly to output.log
    os.exec("grep error", input=r, output=w)
    
    print("Filtered log:", out_file.read_text().strip())
```
