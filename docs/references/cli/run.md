---
title: "kite run"
description: "Execute a starkite script"
weight: 1
---

Execute a script file, a module directory, or an installed module.

## Usage

```bash
kite run ./script.star [flags]    # a loose script file
kite run ./dir                    # a module directory (runs its main.star)
kite run namespace/name           # an installed module (newest revision)
kite run namespace/name@rev       # a specific installed revision
kite <target>                     # shorthand (run is implicit)
./script.star                     # via shebang: #!/usr/bin/env kite
```

Filesystem references require a path prefix (`./`, `../`, or `/`); a bare reference is a module identity. `kite run script.star` errors with a hint to use `./script.star`.

## Run targets

| Target | Resolves to | Requires |
|--------|-------------|----------|
| `./script.star` | the file itself | — (top-level code runs; `main()` optional) |
| `./dir` | `dir/main.star` | a `mod.yaml` manifest **and** a `main()` entry point |
| `namespace/name` | the newest installed revision's `main.star` | the module installed via `kite module install`; a `main()` entry point |
| `namespace/name@rev` | the named revision's `main.star` | that revision installed (full id or unambiguous prefix) |

A directory module or `namespace/name` is **executable** only if its `main.star` defines `main()`. A module without `main()` is a library (loaded via `load()`), and running it directly errors. A loose script file needs no `main()`.

## Variable Injection

Priority order, highest first:

1. **CLI flags:** `--var key=value`
2. **Variable files:** `--var-file=values.yaml`
3. **Default config:** `~/.starkite/config.yaml`
4. **Environment:** `STARKITE_VAR_key=value`
5. **Script default:** `var_str("key", "default")`

## Examples

```bash
# Basic execution
kite run ./deploy.star

# With variables
kite ./deploy.star --var image_tag=v1.0.0 --var replicas=3

# With variable files
kite ./deploy.star --var-file=prod.yaml

# Pipe output
kite ./manifest.star | kubectl apply -f -

# Local filesystem and environment only
kite ./deploy.star --permissions=allow-fs

# OS-level sandbox (Linux only)
kite ./deploy.star --sandbox             # default profile (network ok, no $HOME)
kite ./deploy.star --sandbox=opaque      # offline, $CWD-only
kite ./deploy.star --sandbox --permissions=allow-fs   # both layers
```

For shebang scripts (`./script.star` via `#!/usr/bin/env kite`), set
`STARKITE_SECURITY_SANDBOX` instead of passing `--sandbox`. See the
[Sandbox guide](../../fundamentals/security/sandbox.md) for profile details.
