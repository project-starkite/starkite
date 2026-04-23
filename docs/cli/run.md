---
title: "kite run"
description: "Execute a starkite script"
weight: 1
---

Execute a `.star` script file.

## Usage

```bash
kite run <script.star> [flags]
kite <script.star>          # shorthand (run is implicit)
./script.star               # via shebang: #!/usr/bin/env kite
```

## Variable Injection

Variables can be injected with the following priority (highest to lowest):

1. **CLI flags:** `--var key=value`
2. **Variable files:** `--var-file=values.yaml`
3. **Default config:** `~/.starkite/config.yaml`
4. **Environment:** `STARKITE_VAR_key=value`
5. **Script default:** `var_str("key", "default")`

## Examples

```bash
# Basic execution
kite run deploy.star

# With variables
kite deploy.star --var image_tag=v1.0.0 --var replicas=3

# With variable files
kite deploy.star --var-file=prod.yaml

# Pipe output
kite manifest.star | kubectl apply -f -

# Sandbox mode
kite deploy.star --sandbox
```
