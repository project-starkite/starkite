---
title: "Fundamentals"
description: "The foundation: editions, security, language, modules, embedding"
weight: 20
---

# Fundamentals

The five things every starkite user touches: how the editions fit together, how scripts are secured, how the language works, how modules are organized, and how to embed the runtime.

- [Editions](editions.md) — the four binaries (`kite`, `kitecmd`, `kitecloud`, `kiteai`) and what each one ships
- [Security](security/index.md) — the two-layer model: permissions + sandbox
- [Language](language.md) — variable injection and the `try_` error pattern
- [Modules](modules.md) — auto-loaded modules, `load()`, and WASM plugins
- [Embedding](embedding.md) — using the starkite runtime as a library in your own Go program
