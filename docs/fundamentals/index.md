---
title: "Fundamentals"
description: "The foundation: editions, security, language, modules, embedding"
weight: 20
---

# Fundamentals

Five concepts that govern every starkite script: editions, security, language, modules, embedding.

<div class="grid cards" markdown>

-   :material-cube-outline:{ .lg .middle } __Editions__

    ---

    The four binaries (`kite`, `kitecmd`, `kitecloud`, `kiteai`) and what each one ships.

    [:octicons-arrow-right-24: Read more](editions.md)

-   :material-shield-lock:{ .lg .middle } __Security__

    ---

    The two-layer model: permission rules and the optional gVisor sandbox.

    [:octicons-arrow-right-24: Read more](security/index.md)

-   :material-code-braces:{ .lg .middle } __Language__

    ---

    Variable injection (`var_str`, `--var`, env, config) and the `try_` error pattern.

    [:octicons-arrow-right-24: Read more](language.md)

-   :material-package-variant:{ .lg .middle } __Modules__

    ---

    Auto-loaded built-ins, `load()` for script modules, and WebAssembly plugins.

    [:octicons-arrow-right-24: Read more](modules.md)

-   :material-puzzle:{ .lg .middle } __Embedding__

    ---

    Drive the starkite runtime from a Go program — use Starlark for the parts that benefit from scripting.

    [:octicons-arrow-right-24: Read more](embedding.md)

</div>
