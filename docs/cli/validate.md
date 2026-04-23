---
title: "kite validate"
description: "Validate script syntax without executing"
weight: 22
---

Validate one or more starkite scripts for syntax errors without executing them. Useful for CI/CD pipelines, pre-commit hooks, and editor integration.

## Usage

```bash
kite validate <script.star> [scripts...]
```

Accepts one or more script file paths. Globs are expanded by the shell.

## Flags

| Flag | Description |
|------|-------------|
| `--json` | Output validation results as JSON |

## Exit codes

- `0` — all scripts are valid
- `1` — one or more scripts have syntax errors
- `2` — file not found or read error

## Examples

### Validate a single script

```bash
kite validate deploy.star
# deploy.star: OK
```

### Validate multiple scripts

```bash
kite validate scripts/*.star
# scripts/deploy.star: OK
# scripts/backup.star: OK
# scripts/broken.star: FAILED (1 errors)
#   scripts/broken.star:14:3: undefined: missing_func
```

### CI pre-commit hook

```bash
#!/bin/sh
# .git/hooks/pre-commit
kite validate $(git diff --cached --name-only --diff-filter=d | grep '\.star$')
```

### Machine-readable output

```bash
kite validate --json broken.star
# [
#   {
#     "file": "broken.star",
#     "valid": false,
#     "errors": [
#       {
#         "file": "broken.star",
#         "line": 14,
#         "column": 3,
#         "message": "undefined: missing_func"
#       }
#     ]
#   }
# ]
```
