---
title: "Overview"
description: "The five concepts behind every starkite script"
weight: 20
---

# Concepts

Five core concepts:

<div class="grid cards" markdown>

-   :material-cube-outline:{ .lg .middle } __Editions__

    ---

    Four binaries (`kite`, `kitecmd`, `kitecloud`, `kiteai`) that share the same language and base modules. Each edition exposes a different privileged-module surface.

    [:octicons-arrow-right-24: Read more](editions.md)

-   :material-shield-check:{ .lg .middle } __Permission__

    ---

    A pure-Go rule engine that gates every privileged module call. Runs in-process on every platform; no kernel features required.

    [:octicons-arrow-right-24: Read more](permission.md)

-   :material-shield-lock:{ .lg .middle } __Sandbox__

    ---

    An optional OS-level isolation layer (gVisor on Linux). Confines what the script process can see — filesystem, network, processes — independent of the permission engine.

    [:octicons-arrow-right-24: Read more](sandbox.md)

-   :material-code-braces:{ .lg .middle } __Language__

    ---

    Starlark with two starkite extensions: a layered variable-injection system and the `try_` prefix for error handling.

    [:octicons-arrow-right-24: Read more](language.md)

-   :material-package-variant:{ .lg .middle } __Modules__

    ---

    Built-in capabilities organized by domain. Auto-loaded into every script; cross-script imports go through `load()`.

    [:octicons-arrow-right-24: Read more](modules.md)

</div>
