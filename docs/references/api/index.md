---
title: "API Reference"
description: "Built-in modules grouped by domain — foundations, network, data, cloud, AI"
weight: 1
---

# API Reference

Starkite exposes Go's standard library as type-safe, scriptable Starlark modules. All modules are **auto-loaded** in every `.star` script — no `import` statement, no `load()` call.

For the auto-loading mechanism and the `try_` error pattern that every function supports, see [Modules](../../fundamentals/modules.md) and [Language](../../fundamentals/language.md).

## Foundations

System primitives, control flow, and built-in utilities every script needs.

<div class="grid cards" markdown>

-   :material-server:{ .lg .middle } [`os`](os.md)

    ---

    Environment, process info, command execution.

-   :material-folder-multiple:{ .lg .middle } [`fs`](fs.md)

    ---

    Filesystem operations and `Path` objects.

-   :material-format-text:{ .lg .middle } [`fmt`](fmt.md)

    ---

    Formatted printing — `printf`, `sprintf`, `errorf`.

-   :material-keyboard:{ .lg .middle } [`io`](io.md)

    ---

    Interactive user input — `confirm`, `prompt`.

-   :material-variable:{ .lg .middle } [`vars`](vars.md)

    ---

    Typed variable access — `var_str`, `var_int`, `var_list`, …

-   :material-information-outline:{ .lg .middle } [`runtime`](runtime.md)

    ---

    Runtime and platform information.

-   :material-text-box:{ .lg .middle } [`log`](log.md)

    ---

    Structured logging with slog backend.

-   :material-clipboard-check:{ .lg .middle } [`test`](test.md)

    ---

    Testing assertions and helpers.

-   :material-server-network:{ .lg .middle } [`fleet`](fleet.md)

    ---

    Compute resource fleet management, topology querying, and executor targeting.

-   :material-format-list-bulleted:{ .lg .middle } [`inventory`](inventory.md)

    ---

    Inventory management — file, list, filter, group_by, merge.

-   :material-arrow-decision:{ .lg .middle } [`concur`](concur.md)

    ---

    Concurrent execution — map, each, exec, worker pools.

-   :material-restart:{ .lg .middle } [`retry`](retry.md)

    ---

    Retry logic with fixed delay and exponential backoff.

</div>

## Network

Remote services — HTTP and SSH.

<div class="grid cards" markdown>

-   :material-web:{ .lg .middle } [`http`](http.md)

    ---

    HTTP client, server, and URL builder.

-   :material-console-network:{ .lg .middle } [`ssh`](ssh.md)

    ---

    Remote command execution and SCP file transfer.

</div>

## Data

Database connectivity, encoding, serialization, text processing, and value utilities.

<div class="grid cards" markdown>

-   :material-database:{ .lg .middle } [`sql`](sql.md)

    ---

    SQL databases (SQLite, PostgreSQL, MySQL): query, exec, transactions, batch.

-   :material-code-json:{ .lg .middle } [`json`](json.md)

    ---

    JSON encoding, decoding, and file I/O.

-   :material-file-document:{ .lg .middle } [`yaml`](yaml.md)

    ---

    YAML encoding, decoding, and file I/O.

-   :material-table:{ .lg .middle } [`csv`](csv.md)

    ---

    CSV reading, writing, and file I/O.

-   :material-zip-box:{ .lg .middle } [`gzip`](gzip.md)

    ---

    Gzip compression and decompression.

-   :material-folder-zip:{ .lg .middle } [`zip`](zip.md)

    ---

    ZIP archive reading and writing.

-   :material-numeric:{ .lg .middle } [`base64`](base64.md)

    ---

    Base64 encoding and decoding.

-   :material-pound:{ .lg .middle } [`hash`](hash.md)

    ---

    Cryptographic hash functions.

-   :material-format-text-variant:{ .lg .middle } [`strings`](strings.md)

    ---

    String utility functions.

-   :material-regex:{ .lg .middle } [`regexp`](regexp.md)

    ---

    Regular expression matching and replacement.

-   :material-file-replace:{ .lg .middle } [`template`](template.md)

    ---

    Go `text/template` rendering.

-   :material-clock-outline:{ .lg .middle } [`time`](time.md)

    ---

    Time, duration, and arithmetic.

-   :material-identifier:{ .lg .middle } [`uuid`](uuid.md)

    ---

    UUID generation.

-   :material-table-large:{ .lg .middle } [`table`](table.md)

    ---

    ASCII table rendering.

</div>

## Cloud

Kubernetes and cloud-native resource management — **cloud edition** (`kitecloud` or `kite`).

<div class="grid cards" markdown>

-   :material-kubernetes:{ .lg .middle } [`k8s`](k8s.md)

    ---

    Full Kubernetes API — CRUD, kubectl-equivalents, typed constructors, controllers, webhooks.

</div>

## AI

LLM clients and Model Context Protocol — **AI edition** (`kiteai` or `kite`).

<div class="grid cards" markdown>

-   :material-robot:{ .lg .middle } [`ai`](ai.md)

    ---

    Multi-provider LLM client with chat, tools, streaming, and structured output.

-   :material-connection:{ .lg .middle } [`mcp`](mcp.md)

    ---

    Model Context Protocol — `mcp.serve()` server and `mcp.connect()` client.

</div>
