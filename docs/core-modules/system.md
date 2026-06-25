---
title: "System & processes overview"
description: "Overview of the Starkite os module for host environment and process management"
weight: 10
---

# System & processes

The `os` module manages host environments, process contexts, and shell command execution. Scripts use it to query system information, manipulate environment variables, and run external commands as sub-processes.

To explore specific topics, select one of the following sections:

*   **[Launching processes](sys-launch.md)**: Execute local commands, manage timeouts, environments, and handle execution results.
*   **[Process context](sys-context.md)**: Inspect host names, user profiles, working directories, and process IDs.
*   **[Security](sys-security.md)**: Understand the security model, permission profiles, and gVisor sandbox integration for executing system operations.

---

## Quickstart Example

The following script queries basic host details and runs a shell command:

```python
def main():
    # Query basic host and user context
    print("Host:", os.hostname())
    print("User:", os.user.name)

    # Execute a simple local command
    info = os.exec("uname -a")
    print("OS Info:", info.strip())
```

---

## See also

*   [`os` API reference](../references/api/os.md)
