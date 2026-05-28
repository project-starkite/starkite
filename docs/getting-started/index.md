---
title: "Overview"
description: "Starkite — a secure runtime for cloud-native and agentic AI automation"
weight: 1
---

# Overview

Starkite (pronounced *"star-kite"*) is a secure runtime for cloud-native and agentic-AI automation. Scripts run inside a single binary that bundles a module library, a permission engine, and an optional gVisor sandbox.

## What is Starlark?

Starkite scripts are written in [Starlark](https://github.com/bazelbuild/starlark) — a small, Python-derived language used by the [Bazel](https://bazel.build) build system. Starlark removes constructs that make programs hard to predict or sandbox: no `while` loops, no recursion, no mutable globals after initialization, no classes, no in-language I/O, no catchable exceptions. The result: scripts terminate, behave identically across runs, and expose every privileged operation to the [permission engine](../concepts/permission.md).

Available without import: functions, lambdas, `if`/`for`, comprehensions, string/list/dict/tuple types, `fail()`, and a standard library of `len()`, `range()`, `sorted()`, etc.

For the deep dive, see [Language](../concepts/language.md) and the [Starlark spec](https://github.com/bazelbuild/starlark/blob/master/spec.md).

## Editions

The default binary is **`kite`**, the all-in-one edition — it bundles every module and is what these guides use. The lean editions are smaller subsets for space-conscious targets:

| Binary | Modules | Use when |
|---|---|---|
| `kite` | base + Kubernetes + GenAI/MCP (all-in-one) | **the default** — everything in one binary |
| `kitecmd` | base only | system scripts, CI tasks, general automation |
| `kitecloud` | base + Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kiteai` | base + LLM clients + MCP server/client | agentic AI tools and orchestration |

Start with `kite`. Reach for a lean edition only when binary size or attack surface matters. See [Editions](../concepts/editions.md) for the full picture.

## Where next

- [Install](install.md) — build from source, pull a release, or run the container image
- [Quickstarts](quickstarts/first-script.md) — short capability demos: HTTP, exec, SSH, CLI, sandbox, Kubernetes, MCP
- [Concepts](../concepts/index.md) — editions, security model, language, modules, embedding
- [References](../references/index.md) — CLI and API catalogs
