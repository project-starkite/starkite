---
title: "kite init"
description: "Initialize a new starkite project"
weight: 5
---

Initialize a new starkite project with configuration files and optional templates.

## Usage

```bash
kite init [directory] [flags]
```

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-t, --template name` | Project template | `basic` |
| `--list-templates` | List available templates | |

## Templates

| Template | Description |
|----------|-------------|
| `basic` | Minimal `config.yaml` only |
| `deployment` | SSH deployment script with inventory |
| `kubernetes` | Kubernetes manifest generation |
| `backup` | Remote backup collection script |

## Examples

```bash
kite init                            # Current directory, basic template
kite init ./my-project               # Specific directory
kite init --template=deployment      # Deployment template
kite init --template=kubernetes      # Kubernetes template
```
