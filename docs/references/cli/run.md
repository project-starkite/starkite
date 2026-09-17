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

## Script Arguments and Flags

Scripts declare CLI options using the [`args`](../api/args.md) module (`args.string`, `args.int`, `args.bool`, `args.list`, `args.positional`). Unrecognized flags and positionals passed after the target script are forwarded directly to the script:

```bash
kite run ./deploy.star prod-cluster --action install --replicas 3
```

### Script Help (`--help` / `-h`)

When `--help` or `-h` is passed after the script target, Starkite routes help generation to the script's declared `args` schema and exits cleanly with code `0`:

```bash
kite run ./deploy.star --help
```

To display help for the `kite run` command itself, pass `--help` before the target (e.g., `kite run --help`).

### Runtime Flag Collision and `--` Delimiter

Kite binary runtime flags (e.g., `--dry-run`, `--timeout`, `--permissions`) are evaluated by the Go runner before reaching the script. To forward colliding flag names directly to the script, supply the POSIX `--` delimiter:

```bash
# Sets kite execution timeout to 15m, while passing --timeout 5s and --dry-run to deploy.star:
kite run --timeout 15m ./deploy.star -- --timeout 5s --dry-run
```

### Strict Unhandled Argument Check

If command-line arguments are provided to a script that does not call `args.parse()`, execution halts immediately after the script finishes with exit code `6` (`ExitUsageError`).

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

# OS-level sandbox isolation
kite ./deploy.star --sandboxed                               # default profile (network ok, no $HOME)
kite ./deploy.star --sandbox-opaque                          # offline, $CWD-only
kite ./deploy.star --sandbox-profile=opaque --sandbox-driver=podman # run inside Podman container
kite ./deploy.star --sandboxed --permissions=allow-fs        # both layers
```

For shebang scripts (`./script.star` via `#!/usr/bin/env kite`), set
`STARKITE_SANDBOX_PROFILE` and `STARKITE_SANDBOX_DRIVER` instead of passing CLI flags. See the
[Sandbox guide](../../fundamentals/security/sandbox.md) for profile details.
