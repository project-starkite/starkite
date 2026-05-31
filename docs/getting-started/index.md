---
title: "Welcome to Starkite"
description: "Starkite — a secure scripting runtime for operational and agentic automation"
weight: 1
---

# Welcome to Starkite

Starkite is a scripting runtime for creating system and infrastructure automation using the Starlark language. It is designed to be safe-by-default with support for operational and agentic AI workloads. Starkite  bundles a complete set of standard library modules, a permission engine, and a sandbox system for contained script execution. 

## What is Starlark?

Starkite scripts are written in [Starlark](https://github.com/bazelbuild/starlark) — a small, Python-derived language used by the [Bazel](https://bazel.build) build system. While scripts looks like Python, Starlark removes serveral key language constructs: no `while` loops, no recursion, no mutable globals after initialization, no classes, no catchable exceptions. This allows script to be more predictable and behave identically across runs.

For a deep dive, see [Language](../fundamentals/language.md) and the [Starlark spec](https://github.com/bazelbuild/starlark/blob/master/spec.md).

## Where next

<div class="grid cards" markdown>

-   :material-download:{ .lg .middle } __Install__

    ---

    Build from source, pull a release, or run the container image.

    [:octicons-arrow-right-24: Install](install.md)

-   :material-rocket-launch:{ .lg .middle } __First script__

    ---

    Write and run your first Starkite script.

    [:octicons-arrow-right-24: First script](first-script.md)

-   :material-lightbulb-outline:{ .lg .middle } __Fundamentals__

    ---

    Language, modules, configuration, security, testing, editions.

    [:octicons-arrow-right-24: Fundamentals](../fundamentals/language.md)

-   :material-cog:{ .lg .middle } __Core modules__

    ---

    System, logging, files, JSON/YAML, SSH, HTTP.

    [:octicons-arrow-right-24: Core modules](../core-modules/system.md)

-   :material-book-open-variant:{ .lg .middle } __References__

    ---

    CLI and API catalogs.

    [:octicons-arrow-right-24: References](../references/index.md)

</div>
