---
title: "kite exec"
description: "Execute inline Starlark code"
weight: 2
---

Execute inline Starlark code from the command line.

## Usage

```bash
kite exec '<code>'
```

## Examples

```bash
# Print a value
kite exec 'print("Hello from starkite!")'

# Use modules
kite exec 'print(json.encode({"key": "value"}))'

# Run a command
kite exec 'r = exec("hostname"); print(r.stdout)'

# Check time
kite exec 'print(time.now().string("datetime"))'
```
