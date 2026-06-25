---
title: "Welcome to Starkite"
description: "Starkite — a secure scripting runtime for operational and agentic automation"
weight: 1
---

# Welcome to Starkite

Starkite is a runtime for automating systems and infrastructure with scripts written in [Starlark](https://github.com/bazelbuild/starlark) — a small, deterministic dialect of Python created for [Bazel](https://bazel.build). It targets the jobs that otherwise turn into fragile shell scripts or over-privileged Python: deploying to Kubernetes, running commands over SSH, calling HTTP APIs, or giving an AI agent a controlled set of actions.

## Why Starlark?

Starlark will look familiar — it reads like Python, so if you already know Python you can read a Starkite script on sight. The difference is what it leaves out: no `while` loops, no recursion, no classes, and no mutable global state. Those constraints are what let a script run the same way every time, which is also what makes it safe to run code you didn't write yourself.

See [Language](../fundamentals/language.md) for the full picture, or the [Starlark spec](https://github.com/bazelbuild/starlark/blob/master/spec.md) for the language definition.

## Next Steps

Install the runtime and write your first script. When you want to understand how the pieces fit together, the fundamentals come next.

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

-   :material-kubernetes:{ .lg .middle } __Infrastructure__

    ---

    Automate Kubernetes clusters: deploy resources, write controllers, and handle admission webhooks.

    [:octicons-arrow-right-24: Infrastructure](../infra/k8s-connect.md)

-   :material-robot:{ .lg .middle } __AI Support__

    ---

    Build multi-turn agents, tool-calling loops, and MCP servers.

    [:octicons-arrow-right-24: AI Support](../ai/agents.md)

-   :material-book-open-variant:{ .lg .middle } __References__

    ---

    CLI and API catalogs.

    [:octicons-arrow-right-24: References](../references/index.md)

</div>
