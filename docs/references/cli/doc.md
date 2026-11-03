---
title: "doc"
description: "Inspect runtime symbols, reference documentation, and generate agent skills"
weight: 13
keywords: [kite doc, doc, documentation, skills, agent, reference, symbols, json]
---

# kite doc

Inspect live reference documentation for Starkite modules, functions, and types directly in your terminal, or generate structured skills definitions for autonomous AI coding agents.

The documentation is read directly from the runtime's symbol catalog and API reference tables, ensuring that descriptions, signatures, and return contracts match the compiled binary.

```bash
kite doc [module[.symbol]] [flags]
```

## Available Modes

`kite doc` supports three primary modes:

| Mode | Command | Output |
|---|---|---|
| **Catalog Overview** | `kite doc` | Lists all available modules and predeclared top-level builtins |
| **Module / Symbol Reference** | `kite doc <module>[.<symbol>]` | Renders function signatures, parameters, return types, and docstrings |
| **Agent Skills Generation** | `kite doc --skills` | Generates self-contained, 3-tier `SKILL.md` documents for AI coding agents |

---

## Flags

| Flag | Description | Default |
|---|---|---|
| `--skills` | Generate structured agent skill documentation (`SKILL.md`) following the 3-tier progressive disclosure model | `false` |
| `--category <cat>` | Scope agent skill generation to a functional category (`cloud`, `k8s`, `data`, `sys`) | `""` |
| `--json` | Output documentation metadata in machine-readable JSON format | `false` |

---

## Terminal Reference Documentation

### Discovering Available Modules

Running `kite doc` with no positional arguments displays every standard module registered in the active binary alongside its summary description:

```bash
kite doc
```

Output lists all modules (such as `fs`, `http`, `k8s`, `os`, `strings`, `concur`) and all top-level predeclared globals (such as `path`, `print`, `var_str`, `assert`).

### Inspecting a Module

Pass a module name to view all functions, types, and safe `try_` Result variants provided by that module:

```bash
kite doc http
kite doc fs
```

Each listing includes parameter signatures with default values, return type contracts, and concise descriptions.

### Inspecting a Specific Function or Symbol

Target a specific function or exported method by joining the module name and symbol with a dot or space:

```bash
kite doc http.get
kite doc fs.path
kite doc os exec
```

To view documentation for a predeclared global builtin:

```bash
kite doc print
kite doc read_text
```

### Machine-Readable JSON Output

Pass the `--json` flag to inspect symbol metadata programmatically:

```bash
kite doc fs --json
kite doc http.get --json
```

---

## Agent Skills Generation (`--skills`)

Autonomous AI coding agents (such as Claude Code, Cursor, Antigravity, and Robo) require structured instructions to write idiomatic, sandboxed scripts. Passing `--skills` instructs `kite doc` to synthesize the live runtime catalog into a self-contained `SKILL.md` specification.

### Unified Automation Skill

Generate the comprehensive automation DSL skill covering core invariants, file I/O, subprocess execution, concurrency, and reference tables for all standard modules:

```bash
kite doc --skills > .agent/skills/starkite/SKILL.md
```

### Domain-Scoped Module Skills

To generate a skill focused exclusively on a single domain (such as Kubernetes operations), pass the module name with `--skills`:

```bash
kite doc k8s --skills > .agent/skills/starkite-k8s/SKILL.md
```

### Categorized Skills

To generate a skill spanning related functional areas, pass the `--category` flag:

```bash
# Cloud, HTTP, database, and concurrent pipeline workflows:
kite doc --skills --category=cloud > .agent/skills/starkite-cloud/SKILL.md
```

Available categories include:
- `cloud`: HTTP APIs, Kubernetes, concurrency pipelines, SQL, and containers.
- `k8s`: Dedicated Kubernetes cluster administration and triage.
- `sys`: Operating system commands, filesystem operations, and process execution.
- `data`: JSON, YAML, CSV, template formatting, compression, and hashing.

---

## Exit Codes

| Code | Meaning |
|---|---|
| `0` | Documentation rendered or skill generated successfully |
| `1` | Symbol not found or conflicting flags passed (e.g. `--skills` with `--json`) |
